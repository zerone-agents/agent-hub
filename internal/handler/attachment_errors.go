package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"control-panel/internal/domain/chat"

	"github.com/gin-gonic/gin"
)

// attachmentHTTPStatus maps a stable attachment error code to its HTTP status.
func attachmentHTTPStatus(code string) int {
	switch code {
	case chat.ErrCodeUploadLimitExceeded:
		return http.StatusRequestEntityTooLarge
	case chat.ErrCodeRuntimeAttachmentUnsupported:
		return http.StatusNotImplemented
	case chat.ErrCodeGenerationMismatch:
		// runtime v2.7.0 原子代次校验不通过：容器已重建，附件随旧代销毁。
		return http.StatusPreconditionFailed
	case chat.ErrCodeGenerationUnavailable:
		// runtime 无法确定自身容器身份（代次未知），与空 containerID 同语义。
		return http.StatusServiceUnavailable
	default: // invalid_multipart / attachment_missing / invalid_attachment
		return http.StatusBadRequest
	}
}

// respondAttachmentError maps err into the envelope; non-AttachmentError gets
// a neutral 500 (details stay out of the response body).
func respondAttachmentError(c *gin.Context, err error) {
	var attErr *chat.AttachmentError
	if errors.As(err, &attErr) {
		respondErrorCode(c, attachmentHTTPStatus(attErr.Code), attErr.Code, attErr.Message)
		return
	}
	// Details of non-domain errors stay out of the response body but are
	// logged at this aggregation point so production 500s remain diagnosable.
	log.Printf("attachment upload failed: %v", err)
	respondError(c, http.StatusInternalServerError, "上传失败，请稍后重试")
}

// runtimeAttachmentCode parses a runtime error body {"error","code"} and
// reports whether code is an attachment domain code hub re-exposes verbatim.
func runtimeAttachmentCode(body string) (string, bool) {
	var payload struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return "", false
	}
	switch payload.Code {
	case chat.ErrCodeAttachmentMissing, chat.ErrCodeInvalidAttachment, chat.ErrCodeUploadLimitExceeded,
		chat.ErrCodeGenerationMismatch, chat.ErrCodeGenerationUnavailable:
		return payload.Code, true
	}
	return "", false
}

// attachmentCodeMessage maps an allowlisted attachment-domain code (parsed
// at the runtime client boundary from StreamRun errors, or here from a raw
// upload *http.Response) onto a fixed neutral Chinese message (CONTRIBUTING
// Standards 1: raw runtime text never reaches the response envelope). The
// function stays pure — callers log the raw body first and then use the
// return value; the SendMessage relay, the upload-result mapping, and the
// content proxy share it so all surfaces stay in sync.
func attachmentCodeMessage(code string) string {
	switch code {
	case chat.ErrCodeAttachmentMissing:
		return "附件已失效，请重新上传"
	case chat.ErrCodeGenerationMismatch:
		// runtime v2.7.0 X-Expected-Container-Id 原子校验不通过（412）：
		// 部署代次已变更，本地文件需重新上传后再发送。
		return "附件已过期（部署代次变更），请重新上传"
	case chat.ErrCodeGenerationUnavailable:
		return "Runtime 部署状态异常，请稍后重试"
	case chat.ErrCodeInvalidAttachment:
		return "附件信息无效"
	case chat.ErrCodeUploadLimitExceeded:
		return "附件大小超出限制"
	default: // 非白名单码不会到达（client 边界 allowlist），防御性兜底
		return "附件信息无效"
	}
}
