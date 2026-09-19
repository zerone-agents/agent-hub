package run

import "errors"

var (
	ErrNotFound          = errors.New("run not found")
	ErrInvalidTransition = errors.New("invalid run status transition")
	ErrFrozen            = errors.New("run is completed or archived and cannot be changed")
	ErrConflict          = errors.New("state revision conflict")
	ErrStateNotFound     = errors.New("run state not found")
	ErrSchemaNotFound    = errors.New("state schema not found")
	ErrDuplicate         = errors.New("resource already exists")
	// ErrNoActiveRun marks callers that are not bound to a running Run session
	// (H6 persona tools are only meaningful inside run context).
	ErrNoActiveRun = errors.New("仅运行会话可用")
)
