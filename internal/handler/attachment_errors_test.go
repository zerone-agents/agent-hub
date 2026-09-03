package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/domain/chat"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRespondErrorCode_Shape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	respondErrorCode(c, http.StatusRequestEntityTooLarge, chat.ErrCodeUploadLimitExceeded, "too many files")
	require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	require.JSONEq(t, `{"success":false,"error":"too many files","code":"upload_limit_exceeded"}`, w.Body.String())
}

func TestAttachmentHTTPStatus(t *testing.T) {
	require.Equal(t, http.StatusBadRequest, attachmentHTTPStatus(chat.ErrCodeInvalidMultipart))
	require.Equal(t, http.StatusBadRequest, attachmentHTTPStatus(chat.ErrCodeAttachmentMissing))
	require.Equal(t, http.StatusBadRequest, attachmentHTTPStatus(chat.ErrCodeInvalidAttachment))
	require.Equal(t, http.StatusRequestEntityTooLarge, attachmentHTTPStatus(chat.ErrCodeUploadLimitExceeded))
	require.Equal(t, http.StatusNotImplemented, attachmentHTTPStatus(chat.ErrCodeRuntimeAttachmentUnsupported))
}

func TestRespondAttachmentError_MapsDomainError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	respondAttachmentError(c, chat.NewAttachmentError(chat.ErrCodeUploadLimitExceeded, "limit hit"))
	require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	require.Contains(t, w.Body.String(), `"code":"upload_limit_exceeded"`)
}

func TestRespondAttachmentError_NeutralForUnknownError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	respondAttachmentError(c, assertErr("boom"))
	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.NotContains(t, w.Body.String(), "boom")
}

func TestRuntimeAttachmentCode(t *testing.T) {
	_, ok := runtimeAttachmentCode(`{"error":"Attachment not found","code":"attachment_missing","path":".zerone-uploads/x"}`)
	require.True(t, ok)
	_, ok = runtimeAttachmentCode(`{"error":"Unauthorized","reason":"invalid api key"}`)
	require.False(t, ok)
	_, ok = runtimeAttachmentCode(`not json`)
	require.False(t, ok)
}

func TestRuntimeErrorMessage(t *testing.T) {
	require.Equal(t, "附件已失效，请重新上传", runtimeErrorMessage(chat.ErrCodeAttachmentMissing))
	require.Equal(t, "附件数量或大小超出限制", runtimeErrorMessage(chat.ErrCodeUploadLimitExceeded))
	require.Equal(t, "附件信息无效", runtimeErrorMessage(chat.ErrCodeInvalidAttachment))
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
