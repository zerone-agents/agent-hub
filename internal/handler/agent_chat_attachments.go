// internal/handler/agent_chat_attachments.go
package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// Hub-side upload limits, enforced INDEPENDENTLY of the runtime (issue #94
// acceptance: hub 不依赖 runtime 的限额；两边保持同值 10 / 20MB / 50MB）。
const (
	uploadMaxFiles      = 10
	uploadMaxFileBytes  = 20 << 20
	uploadMaxTotalBytes = 50 << 20
)

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
		respondError(c, http.StatusNotFound, "session not found")
		return
	}
	if sess.AgentID != agentName {
		respondError(c, http.StatusNotFound, "session not found")
		return
	}
	baseURL, apiKey, err := h.svc.ResolveRuntime(tenantID, agentName)
	if err != nil {
		respondError(c, http.StatusConflict, "agent not available: "+err.Error())
		return
	}
	if !h.svc.AttachmentsSupportedAt(c.Request.Context(), baseURL) {
		respondErrorCode(c, http.StatusNotImplemented, chat.ErrCodeRuntimeAttachmentUnsupported,
			"runtime does not support attachments (requires >= 2.5.0)")
		return
	}

	mediaType, params, err := mime.ParseMediaType(c.Request.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		respondErrorCode(c, http.StatusBadRequest, chat.ErrCodeInvalidMultipart, "Content-Type must be multipart/form-data")
		return
	}
	if params["boundary"] == "" {
		respondErrorCode(c, http.StatusBadRequest, chat.ErrCodeInvalidMultipart, "missing multipart boundary")
		return
	}

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
			h.respondUploadResult(c, res.resp)
			return
		}
		// runtime 未产生可透传的响应（连接失败/超时等传输错误）。
		if res.err != nil {
			respondError(c, http.StatusBadGateway, "runtime unreachable: "+res.err.Error())
			return
		}
		respondError(c, http.StatusBadGateway, "runtime unreachable")
		return
	}
	_ = pw.Close()

	res := <-done
	if res.err != nil {
		respondError(c, http.StatusBadGateway, "runtime unreachable: "+res.err.Error())
		return
	}
	defer res.resp.Body.Close()
	h.respondUploadResult(c, res.resp)
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
			return chat.NewAttachmentError(chat.ErrCodeInvalidMultipart, "malformed multipart: "+err.Error())
		}
		if part.FileName() == "" {
			continue // 非文件字段：忽略
		}
		files++
		if files > uploadMaxFiles {
			return chat.NewAttachmentError(chat.ErrCodeUploadLimitExceeded,
				fmt.Sprintf("too many files: limit is %d", uploadMaxFiles))
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
		n, err := io.Copy(dst, io.LimitReader(part, uploadMaxFileBytes+1))
		if err != nil {
			// 同上：io.Copy 的错误可能来自读源（客户端断开）或写目标
			// （runtime 断连/提前关闭 body）；两者都不是客户端 multipart
			// 解析错误，原样返回避免被误映射成 400 invalid_multipart。
			return err
		}
		if n > uploadMaxFileBytes {
			return chat.NewAttachmentError(chat.ErrCodeUploadLimitExceeded,
				fmt.Sprintf("file %q exceeds the 20MB single-file limit", part.FileName()))
		}
		totalBytes += n
		if totalBytes > uploadMaxTotalBytes {
			return chat.NewAttachmentError(chat.ErrCodeUploadLimitExceeded,
				"total upload size exceeds the 50MB request limit")
		}
	}
	if err := mw.Close(); err != nil {
		// 同上：收尾 boundary 写入 pipe 失败是传输层错误，非客户端域错误。
		return err
	}
	return nil
}

// respondUploadResult maps the runtime upload response onto the hub envelope.
func (h *AgentChatHandler) respondUploadResult(c *gin.Context, resp *http.Response) {
	switch {
	case resp.StatusCode == http.StatusCreated:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		var parsed struct {
			Files []services.AttachmentDesc `json:"files"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil || len(parsed.Files) == 0 || len(parsed.Files) > uploadMaxFiles {
			respondError(c, http.StatusBadGateway, "unexpected runtime upload response")
			return
		}
		for _, f := range parsed.Files {
			if err := services.ValidateAttachmentDesc(f); err != nil {
				respondError(c, http.StatusBadGateway, "unexpected runtime upload response")
				return
			}
		}
		respondCreated(c, gin.H{"files": parsed.Files})
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed:
		respondErrorCode(c, http.StatusNotImplemented, chat.ErrCodeRuntimeAttachmentUnsupported,
			"runtime does not support attachments (requires >= 2.5.0)")
	default:
		bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 8192))
		body := string(bodyBytes)
		if code, ok := runtimeAttachmentCode(body); ok {
			respondErrorCode(c, attachmentHTTPStatus(code), code, runtimeErrorMessage(body))
			return
		}
		// mid-stream abort（runtime 未排空 body 即回响应后关闭连接）：
		// transport 在请求体写失败时会把已收到的响应体标记为关闭
		// （"http: read on closed response body"），code 无法从 body 解析。
		// runtime 契约中 413 唯一对应 upload_limit_exceeded（见
		// attachmentHTTPStatus 的双向映射），按状态码兜底保持透传语义。
		if readErr != nil && resp.StatusCode == http.StatusRequestEntityTooLarge {
			respondErrorCode(c, http.StatusRequestEntityTooLarge, chat.ErrCodeUploadLimitExceeded,
				"runtime upload rejected the request (limit exceeded)")
			return
		}
		respondError(c, http.StatusBadGateway, fmt.Sprintf("runtime upload failed: HTTP %d", resp.StatusCode))
	}
}

// AttachmentContent streams an attachment's bytes from the runtime through a
// session-scoped authenticated proxy (history image preview / file download).
// GET /api/v1/agents/:name/chat/sessions/:id/attachments/content?path=...
//
// runtime /v1/files/content 本身能读整个工作区——本端点因此必须做双层
// 收敛：path 语法限定 .zerone-uploads/ 扁平 + 交叉核验该 path 曾在本
// session 的消息里持久化过。
func (h *AgentChatHandler) AttachmentContent(c *gin.Context) {
	agentName := services.NormalizeAgentName(c.Param("name"))
	sessionID := c.Param("id")
	userID := c.MustGet("user_id").(string)
	tenantID := tenant.GetTenantID(c)
	pathParam := c.Query("path")

	sess, err := h.svc.GetSession(tenantID, userID, sessionID)
	if err != nil {
		respondError(c, http.StatusNotFound, "session not found")
		return
	}
	if sess.AgentID != agentName {
		respondError(c, http.StatusNotFound, "session not found")
		return
	}
	if err := services.ValidateUploadsPath(pathParam); err != nil {
		respondErrorCode(c, http.StatusBadRequest, chat.ErrCodeInvalidAttachment, err.Error())
		return
	}
	known, err := h.svc.SessionHasAttachment(tenantID, userID, sessionID, pathParam)
	if err != nil || !known {
		respondError(c, http.StatusNotFound, "attachment not found")
		return
	}
	baseURL, apiKey, err := h.svc.ResolveRuntime(tenantID, agentName)
	if err != nil {
		respondError(c, http.StatusConflict, "agent not available: "+err.Error())
		return
	}

	resp, err := h.svc.RuntimeClient().ProxyFiles(c.Request.Context(), http.MethodGet, baseURL, apiKey,
		"/v1/files/content?path="+url.QueryEscape(pathParam), "")
	if err != nil {
		respondError(c, http.StatusBadGateway, "runtime unreachable: "+err.Error())
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
		respondError(c, http.StatusNotFound, "temporary file no longer available")
	default:
		respondError(c, http.StatusBadGateway, fmt.Sprintf("runtime file fetch failed: HTTP %d", resp.StatusCode))
	}
}
