package scene

import (
	"errors"
	"fmt"
)

var (
	ErrSceneNotFound = errors.New("场景不存在")
	ErrSceneExists   = errors.New("场景已存在")
	ErrAgentNotFound = errors.New("关联的 Agent 不存在")
)

// ValidationError 标记用户面校验错误（请求参数/领域规则不合法）。
type ValidationError struct {
	msg string
}

func (e *ValidationError) Error() string { return e.msg }

func NewValidationErrorf(format string, args ...any) error {
	return &ValidationError{msg: fmt.Sprintf(format, args...)}
}
