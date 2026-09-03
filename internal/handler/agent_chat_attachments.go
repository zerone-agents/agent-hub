// internal/handler/agent_chat_attachments.go
package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/chat"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

// 领域限额（附件 10 个 / 单文件 20MB / 总量 50MB）复用 services 包常量——
// 单一事实来源，与 runtime uploads.ts 保持同值（issue #94 acceptance：hub
// 独立执行限额，不依赖 runtime；两边只是数值一致）。

// 整个 multipart 请求体硬上限（services.MaxAttachmentTotalBytes 之上留
// boundary/headers 余量；请求体工程上限非领域限额，故留在 handler）；
// MaxBytesReader 超限时读错误经 relayMultipart 落 invalid_multipart 400。
var uploadMaxRequestBytes int64 = 60 << 20

// UploadAttachments proxies a multipart upload to the session's runtime.
// POST /api/v1/agents/:name/chat/sessions/:id/uploads
//
// 校验链在读 body 之前完成：tenant（JWT）→ session 归属 → agent 绑定 →
// 部署状态 → runtime 版本门槛。中继全程流式（逐 part，不缓冲整个文件），
// hub 边流边限额。Runtime Token 只在此处注入，绝不经浏览器。
func (h *AgentChatHandler) UploadAttachments(c *gin.Context) {
	agentName := services.NormalizeAgentName(c.Param("name"))
	sessionID := c.Param("id")
	userID := c.MustGet("user_id").(string)
	tenantID := tenant.GetTenantID(c)

	sess, err := h.svc.GetSession(tenantID, userID, sessionID)
	if err != nil {
		log.Printf("[chat] upload session lookup failed: tenant=%s session=%s user=%s err=%v",
			tenantID, sessionID, userID, err)
		respondError(c, http.StatusNotFound, "会话不存在")
		return
	}
	if sess.AgentID != agentName {
		respondError(c, http.StatusNotFound, "会话不存在")
		return
	}
	// containerID 在上传请求发起前取得：记录天然绑定实际处理上传的容器
	// 代次（上传中途重建=连接失败无记录；上传后重建=记录标旧代随后失效
	// ——两个竞态方向都封死，issue #94 review R3）。
	baseURL, apiKey, containerID, err := h.svc.ResolveRuntime(tenantID, agentName)
	if err != nil {
		log.Printf("[chat] upload resolve runtime failed: tenant=%s agent=%s err=%v", tenantID, agentName, err)
		respondError(c, http.StatusConflict, "Agent 暂不可用，请稍后重试")
		return
	}
	// 空 containerID fail-closed（issue #94 review R4 P1-2）：deployer 未报告
	// 容器代次 ⇒ 不发起上传、不落记录。空代次记录会在发送侧对「历史空
	// 记录 + 当前空 ID」判相等放行（fail-open），源头拒绝才能让三个附件
	// 入口语义一致：当前代次未知 ⇒ 附件一律不可用。
	if containerID == "" {
		log.Printf("[chat] upload rejected, empty container generation: tenant=%s agent=%s session=%s",
			tenantID, agentName, sessionID)
		respondError(c, http.StatusServiceUnavailable, "部署状态异常，附件暂不可用，请稍后重试")
		return
	}
	if !h.svc.AttachmentsSupportedAt(c.Request.Context(), baseURL) {
		respondErrorCode(c, http.StatusNotImplemented, chat.ErrCodeRuntimeAttachmentUnsupported,
			"当前 Runtime 版本不支持附件（需 ≥ 2.5.0），请升级 Runtime 并重新部署 Agent")
		return
	}

	mediaType, params, err := mime.ParseMediaType(c.Request.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		log.Printf("[chat] upload rejected content-type: %q err=%v", c.Request.Header.Get("Content-Type"), err)
		respondErrorCode(c, http.StatusBadRequest, chat.ErrCodeInvalidMultipart, "上传请求格式错误（需 multipart/form-data）")
		return
	}
	if params["boundary"] == "" {
		respondErrorCode(c, http.StatusBadRequest, chat.ErrCodeInvalidMultipart, "上传请求缺少 boundary")
		return
	}
	// 整个请求体总量上限：非 file part 虽被 relayMultipart 跳过，但必须读穿
	// 才能定位 boundary，巨型 text field 会造成无界 ingress——MaxBytesReader
	// 在读层面截断（超限读错误在 relayMultipart 中按 malformed multipart 映射）。
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, uploadMaxRequestBytes)

	// 流式中继：io.Pipe 一端接 runtime 请求体，一端由 multipart.Writer 重打包。
	// 绝不调用 gin FormFile/ParseMultipartForm（会整体缓冲）。
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)

	type uploadResult struct {
		resp *http.Response
		err  error
	}
	done := make(chan uploadResult, 1)
	go func() {
		resp, err := h.svc.RuntimeClient().UploadFiles(c.Request.Context(), baseURL, apiKey, pr, mw.FormDataContentType())
		done <- uploadResult{resp: resp, err: err}
	}()

	relayErr := relayMultipart(multipart.NewReader(c.Request.Body, params["boundary"]), mw)
	if relayErr != nil {
		_ = pw.CloseWithError(relayErr) // 中断出站请求体
		res := <-done
		// 域错误（限额/客户端 multipart 解析）优先分类：无论 runtime 是否
		// 已响应，响应都不是给客户端的——若已收到则关闭 body（不再读取），
		// 按域错误原样映射。
		var attErr *chat.AttachmentError
		if errors.As(relayErr, &attErr) {
			if res.err == nil && res.resp != nil {
				_ = res.resp.Body.Close()
			}
			respondAttachmentError(c, relayErr)
			return
		}
		if res.err == nil && res.resp != nil {
			// 传输类裸错误但 runtime 已响应（如 413 mid-stream）：
			// body 保持可读并交由 respondUploadResult 消费（它从中解析
			// runtime code），这里只延迟关闭。切勿提前 Close——否则 body
			// 解析路径变成死代码，非 413 的可解析状态码（如 404→501、
			// 400/500 域码）全部退化为泛化 502。
			defer res.resp.Body.Close()
			h.respondUploadResult(c, res.resp, tenantID, userID, sessionID, containerID)
			return
		}
		// runtime 未产生可透传的响应（连接失败/超时等传输错误）。
		if res.err != nil {
			log.Printf("[chat] upload relay aborted, transport error: session=%s err=%v", sessionID, res.err)
			respondError(c, http.StatusBadGateway, "上传服务暂时不可用")
			return
		}
		respondError(c, http.StatusBadGateway, "上传服务暂时不可用")
		return
	}
	_ = pw.Close()

	res := <-done
	if res.err != nil {
		log.Printf("[chat] upload runtime transport error: session=%s err=%v", sessionID, res.err)
		respondError(c, http.StatusBadGateway, "上传服务暂时不可用")
		return
	}
	defer res.resp.Body.Close()
	h.respondUploadResult(c, res.resp, tenantID, userID, sessionID, containerID)
}

// relayMultipart copies file parts from mr into mw, enforcing the hub-side
// limits while streaming. Non-file form fields are dropped. Incoming part
// headers (Content-Disposition / Content-Type) are forwarded VERBATIM so the
// browser's filename encoding survives the re-package.
func relayMultipart(mr *multipart.Reader, mw *multipart.Writer) error {
	var files, totalBytes int64
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("[chat] malformed multipart body: %v", err)
			return chat.NewAttachmentError(chat.ErrCodeInvalidMultipart, "上传数据格式错误")
		}
		if part.FileName() == "" {
			continue // 非文件字段：忽略
		}
		files++
		if files > services.MaxAttachmentsPerMessage {
			return chat.NewAttachmentError(chat.ErrCodeUploadLimitExceeded,
				fmt.Sprintf("附件最多 %d 个", services.MaxAttachmentsPerMessage))
		}

		hdr := make(textproto.MIMEHeader)
		if cd := part.Header.Get("Content-Disposition"); cd != "" {
			hdr.Set("Content-Disposition", cd)
		} else {
			hdr.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{
				"name": "files", "filename": part.FileName(),
			}))
		}
		if ct := part.Header.Get("Content-Type"); ct != "" {
			hdr.Set("Content-Type", ct)
		}
		dst, err := mw.CreatePart(hdr)
		if err != nil {
			// 写目标（出站 pipe）错误 = transport 已中断请求体（runtime 不可达/
			// 中途终止），不是客户端 multipart 域错误：原样返回，由
			// UploadAttachments 按 res.err/res.resp 分类（502 或透传）。
			return err
		}
		// LimitReader MAX+1 探测：多读一个字节即超限（tool_artifact.go 同款技巧）
		n, err := io.Copy(dst, io.LimitReader(part, services.MaxAttachmentFileBytes+1))
		if err != nil {
			// 同上：io.Copy 的错误可能来自读源（客户端断开）或写目标
			// （runtime 断连/提前关闭 body）；两者都不是客户端 multipart
			// 解析错误，原样返回避免被误映射成 400 invalid_multipart。
			return err
		}
		if n > services.MaxAttachmentFileBytes {
			return chat.NewAttachmentError(chat.ErrCodeUploadLimitExceeded,
				fmt.Sprintf("「%s」超过单文件 %dMB 上限", part.FileName(), services.MaxAttachmentFileBytes>>20))
		}
		totalBytes += n
		if totalBytes > services.MaxAttachmentTotalBytes {
			return chat.NewAttachmentError(chat.ErrCodeUploadLimitExceeded,
				fmt.Sprintf("附件总大小超过 %dMB 上限", services.MaxAttachmentTotalBytes>>20))
		}
	}
	if err := mw.Close(); err != nil {
		// 同上：收尾 boundary 写入 pipe 失败是传输层错误，非客户端域错误。
		return err
	}
	return nil
}

// respondUploadResult maps the runtime upload response onto the hub envelope.
// On 201 it also persists the descriptors as server-side upload records (the
// authorization anchor — issue #94 review F1); tenant/user/session scope the
// records and must come from the request context, never the runtime body.
// containerID is the deployer-reported generation captured before the upload
// request was issued — records are stamped with it so later sends/downloads
// can reject stale generations (issue #94 review R3).
func (h *AgentChatHandler) respondUploadResult(c *gin.Context, resp *http.Response, tenantID, userID, sessionID, containerID string) {
	switch {
	case resp.StatusCode == http.StatusCreated:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		var parsed struct {
			Files []services.AttachmentDesc `json:"files"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil || len(parsed.Files) == 0 || len(parsed.Files) > services.MaxAttachmentsPerMessage {
			log.Printf("[chat] unparseable runtime upload response: session=%s body=%s", sessionID, body)
			respondError(c, http.StatusBadGateway, "上传服务响应异常")
			return
		}
		for _, f := range parsed.Files {
			if err := services.ValidateAttachmentDesc(f); err != nil {
				log.Printf("[chat] invalid descriptor in runtime upload response: session=%s err=%v", sessionID, err)
				respondError(c, http.StatusBadGateway, "上传服务响应异常")
				return
			}
		}
		if err := h.svc.SaveUploadRecords(tenantID, userID, sessionID, containerID, parsed.Files); err != nil {
			log.Printf("[chat] persist upload records failed: tenant=%s session=%s user=%s err=%v",
				tenantID, sessionID, userID, err)
			respondError(c, http.StatusBadGateway, "上传失败，请稍后重试")
			return
		}
		respondCreated(c, gin.H{"files": parsed.Files})
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed:
		respondErrorCode(c, http.StatusNotImplemented, chat.ErrCodeRuntimeAttachmentUnsupported,
			"当前 Runtime 版本不支持附件（需 ≥ 2.5.0），请升级 Runtime 并重新部署 Agent")
	default:
		bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 8192))
		body := string(bodyBytes)
		if code, ok := runtimeAttachmentCode(body); ok {
			log.Printf("[chat] runtime upload rejected: session=%s status=%d code=%s body=%s",
				sessionID, resp.StatusCode, code, body)
			respondErrorCode(c, attachmentHTTPStatus(code), code, runtimeErrorMessage(code))
			return
		}
		// mid-stream abort（runtime 未排空 body 即回响应后关闭连接）：
		// transport 在请求体写失败时会把已收到的响应体标记为关闭
		// （"http: read on closed response body"），code 无法从 body 解析。
		// runtime 契约中 413 唯一对应 upload_limit_exceeded（见
		// attachmentHTTPStatus 的双向映射），按状态码兜底保持透传语义。
		if readErr != nil && resp.StatusCode == http.StatusRequestEntityTooLarge {
			log.Printf("[chat] runtime 413 body unreadable (mid-stream abort): session=%s readErr=%v", sessionID, readErr)
			respondErrorCode(c, http.StatusRequestEntityTooLarge, chat.ErrCodeUploadLimitExceeded,
				"附件数量或大小超出限制")
			return
		}
		log.Printf("[chat] runtime upload failed: session=%s status=%d", sessionID, resp.StatusCode)
		respondError(c, http.StatusBadGateway, "上传服务暂时不可用")
	}
}

// AttachmentContent streams an attachment's bytes from the runtime through a
// session-scoped authenticated proxy (history image preview / file download).
// GET /api/v1/agents/:name/chat/sessions/:id/attachments/content?path=...
//
// runtime /v1/files/content 本身能读整个工作区——本端点因此必须做双层
// 收敛：path 语法限定 .zerone-uploads/ 扁平 + 交叉核验该 path 存在本
// session 的服务端上传记录（上传时落库，不可伪造；消息 file parts 仅展示）。
func (h *AgentChatHandler) AttachmentContent(c *gin.Context) {
	agentName := services.NormalizeAgentName(c.Param("name"))
	sessionID := c.Param("id")
	userID := c.MustGet("user_id").(string)
	tenantID := tenant.GetTenantID(c)
	pathParam := c.Query("path")

	sess, err := h.svc.GetSession(tenantID, userID, sessionID)
	if err != nil {
		log.Printf("[chat] attachment content session lookup failed: tenant=%s session=%s user=%s err=%v",
			tenantID, sessionID, userID, err)
		respondError(c, http.StatusNotFound, "会话不存在")
		return
	}
	if sess.AgentID != agentName {
		respondError(c, http.StatusNotFound, "会话不存在")
		return
	}
	if err := services.ValidateUploadsPath(pathParam); err != nil {
		log.Printf("[chat] attachment content rejected path: session=%s path=%q err=%v", sessionID, pathParam, err)
		respondErrorCode(c, http.StatusBadRequest, chat.ErrCodeInvalidAttachment, "附件信息无效")
		return
	}
	baseURL, apiKey, containerID, err := h.svc.ResolveRuntime(tenantID, agentName)
	if err != nil {
		log.Printf("[chat] attachment content resolve runtime failed: tenant=%s agent=%s err=%v", tenantID, agentName, err)
		respondError(c, http.StatusConflict, "Agent 暂不可用，请稍后重试")
		return
	}
	// 空 containerID 显式早退（issue #94 review R4 P1-2）：SessionHasAttachment
	// 对空代次已失败关闭（下方 known=false → 404），这里在 handler 层显式化，
	// 使三个附件入口的空代次策略一致可见。
	if containerID == "" {
		log.Printf("[chat] attachment content rejected, empty container generation: tenant=%s agent=%s session=%s",
			tenantID, agentName, sessionID)
		respondError(c, http.StatusNotFound, "临时文件已不可用")
		return
	}
	// 部署代次绑定（issue #94 review R3）：上传记录只授权创建它的容器代次
	//（deployer 报告的 container id 精确相等，零时间容差——旧代文件已随
	// 容器销毁，路径可能已被他人同名上传复用）。containerID 为空（deployer
	// 未报告）同样失败关闭——代次未知时宁可 404 不放行。
	known, err := h.svc.SessionHasAttachment(tenantID, userID, sessionID, pathParam, containerID)
	if err != nil {
		log.Printf("[chat] attachment record lookup failed: session=%s path=%q err=%v", sessionID, pathParam, err)
	}
	if err != nil || !known {
		respondError(c, http.StatusNotFound, "附件不存在")
		return
	}

	resp, err := h.svc.RuntimeClient().ProxyFiles(c.Request.Context(), http.MethodGet, baseURL, apiKey,
		"/v1/files/content?path="+url.QueryEscape(pathParam), "")
	if err != nil {
		log.Printf("[chat] attachment content transport error: session=%s path=%q err=%v", sessionID, pathParam, err)
		respondError(c, http.StatusBadGateway, "附件服务暂时不可用")
		return
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusOK:
		for _, h2 := range []string{"Content-Type", "Content-Length", "Content-Disposition"} {
			if v := resp.Header.Get(h2); v != "" {
				c.Header(h2, v)
			}
		}
		c.Status(http.StatusOK)
		_, _ = io.Copy(c.Writer, resp.Body)
	case resp.StatusCode == http.StatusNotFound:
		// runtime 容器重建后文件丢失（附件生命周期 = 容器生命周期）
		respondError(c, http.StatusNotFound, "临时文件已不可用")
	default:
		log.Printf("[chat] attachment content runtime error: session=%s path=%q status=%d",
			sessionID, pathParam, resp.StatusCode)
		respondError(c, http.StatusBadGateway, "附件服务暂时不可用")
	}
}
