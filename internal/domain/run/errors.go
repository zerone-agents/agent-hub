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
)
