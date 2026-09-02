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
	respondError(c, http.StatusInternalServerError, "upload failed")
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

// runtimeErrorMessage extracts the human-readable message from a runtime
// error body, falling back to the raw body (truncated).
func runtimeErrorMessage(body string) string {
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil || payload.Error == "" {
		if len(body) > 200 {
			body = body[:200]
		}
		return body
	}
	return payload.Error
}
