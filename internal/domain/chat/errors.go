package chat

import "errors"

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

// v2 同步状态机哨兵错误（issue #232 H2）。handler 层据此映射 HTTP 状态：
// NotFound→404、NotClaimed/ClaimConflict/ClaimDeleted→409。
var (
	// ErrSyncNotFound：同步键无状态行（未 claim / 异租户视角隔离）。
	ErrSyncNotFound = errors.New("sync state not found")
	// ErrSyncNotClaimed：Apply 前必须完成 claim（无该键不得自行创建覆盖）。
	ErrSyncNotClaimed = errors.New("sync key not claimed")
	// ErrSyncClaimConflict：claim 所有权冲突——异 source 写同键，或 native
	// 试图接管既有 v1 历史（不能用更大版本抢占）。
	ErrSyncClaimConflict = errors.New("sync claim ownership conflict")
	// ErrSyncClaimDeleted：已删除的键（永久 tombstone）不允许 claim。
	ErrSyncClaimDeleted = errors.New("sync key deleted (tombstone)")
	// ErrSyncManagedSession：会话由 v2 同步管理——绕过协议的正文 mutation
	//（CreateMessage/UpdateSessionTitle/DeleteMessageByID/CreateSession）
	// 必须拒绝（issue #232 H3：v2 会话不接受绕过协议的正文变更）。
	ErrSyncManagedSession = errors.New("session managed by sync v2")
)
