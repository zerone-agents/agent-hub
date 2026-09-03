package chat

// Stable attachment error codes (issue #94 contract with the frontend).
const (
	ErrCodeInvalidMultipart             = "invalid_multipart"
	ErrCodeUploadLimitExceeded          = "upload_limit_exceeded"
	ErrCodeRuntimeAttachmentUnsupported = "runtime_attachment_unsupported"
	ErrCodeAttachmentMissing            = "attachment_missing"
	ErrCodeInvalidAttachment            = "invalid_attachment"
	ErrCodeGenerationMismatch           = "generation_mismatch"
	ErrCodeGenerationUnavailable        = "generation_unavailable"
)

// AttachmentError is a structured attachment-domain error carrying a stable
// code. Handlers map it to an HTTP status plus the envelope `code` field.
type AttachmentError struct {
	Code    string
	Message string
}

func (e *AttachmentError) Error() string { return e.Message }

func NewAttachmentError(code, message string) *AttachmentError {
	return &AttachmentError{Code: code, Message: message}
}
