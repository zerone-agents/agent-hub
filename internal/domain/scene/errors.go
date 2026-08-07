package scene

import "errors"

var (
	ErrSceneNotFound = errors.New("场景不存在")
	ErrSceneExists   = errors.New("场景已存在")
	ErrAgentNotFound = errors.New("关联的 Agent 不存在")
)
