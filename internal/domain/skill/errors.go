package skill

import "errors"

var (
	ErrSkillNotFound     = errors.New("技能不存在")
	ErrSkillFileNotFound = errors.New("技能文件不存在")
	ErrInvalidSkillFile  = errors.New("技能文件无效")
	ErrFileTooLarge      = errors.New("文件大小超出限制")
)
