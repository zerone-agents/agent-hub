// internal/handler/agent_chat_attachments.go
package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"

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
		if res := <-done; res.err == nil {
			_ = res.resp.Body.Close()
		}
		respondAttachmentError(c, relayErr)
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
			return chat.NewAttachmentError(chat.ErrCodeInvalidMultipart, "create part: "+err.Error())
		}
		// LimitReader MAX+1 探测：多读一个字节即超限（tool_artifact.go 同款技巧）
		n, err := io.Copy(dst, io.LimitReader(part, uploadMaxFileBytes+1))
		if err != nil {
			return chat.NewAttachmentError(chat.ErrCodeInvalidMultipart, "read part: "+err.Error())
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
		return chat.NewAttachmentError(chat.ErrCodeInvalidMultipart, "close multipart: "+err.Error())
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
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		if code, ok := runtimeAttachmentCode(string(body)); ok {
			respondErrorCode(c, attachmentHTTPStatus(code), code, runtimeErrorMessage(string(body)))
			return
		}
		respondError(c, http.StatusBadGateway, fmt.Sprintf("runtime upload failed: HTTP %d", resp.StatusCode))
	}
}
