package skill

import (
	"errors"
	"fmt"
)

var (
	ErrSkillNotFound     = errors.New("技能不存在")
	ErrSkillFileNotFound = errors.New("技能文件不存在")
	ErrInvalidSkillFile  = errors.New("技能文件无效")
	ErrFileTooLarge      = errors.New("文件大小超出限制")
)

// ValidationError 标记用户面校验错误（请求参数/领域规则不合法）：
// HTTP 边界（respondSkillError）返回 400 + 原文中文文案；与之相对，
// 内部诊断（DB/OSS/zip 等基础设施包装）走 500 中性文案（issue #95 P2）。
type ValidationError struct{ msg string }

func (e *ValidationError) Error() string { return e.msg }

func NewValidationErrorf(format string, args ...any) error {
	return &ValidationError{msg: fmt.Sprintf(format, args...)}
}
