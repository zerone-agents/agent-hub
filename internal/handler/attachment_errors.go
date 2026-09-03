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
	case chat.ErrCodeAttachmentMissing, chat.ErrCodeInvalidAttachment, chat.ErrCodeUploadLimitExceeded:
		return payload.Code, true
	}
	return "", false
}

// runtimeErrorMessage maps a runtime attachment domain code onto a neutral
// Chinese user-facing message (CONTRIBUTING Standards 1: raw runtime text
// never reaches the response envelope). The function stays pure — callers
// log the raw body first and then use the return value; the SendMessage
// relay and the upload-result mapping share it so both surfaces stay in
// sync.
func runtimeErrorMessage(code string) string {
	switch code {
	case chat.ErrCodeAttachmentMissing:
		return "附件已失效，请重新上传"
	case chat.ErrCodeUploadLimitExceeded:
		return "附件数量或大小超出限制"
	default: // invalid_attachment 等其余附件域码
		return "附件信息无效"
	}
}
