package agent

import (
	"errors"
	"fmt"
	"strings"
)

// Tool 领域 sentinel errors（issue #88）。handler 用 errors.Is/As 精确映射
// HTTP 状态码——模式对齐 internal/domain/skill/errors.go。
var (
	ErrToolNotFound        = errors.New("Tool 不存在")
	ErrToolIsBuiltin       = errors.New("内置工具为共享只读记录，不允许修改、补传、下载或删除")
	ErrToolNameExists      = errors.New("Tool 名称已存在")
	ErrInvalidToolFile     = errors.New("工具文件无效：仅支持 .ts / .mts / .js / .mjs 单文件")
	ErrToolFileEmpty       = errors.New("工具文件不能为空")
	ErrToolFileTooLarge    = errors.New("工具文件大小不能超过 5 MiB")
	ErrToolArtifactMissing = errors.New("自定义工具缺少制品文件，请先补传")
	ErrToolStorageDisabled = errors.New("文件存储未配置（OSS），无法上传或下载工具文件")
)

// ToolInUseError 删除保护：仍被 Agent 关联的自定义工具禁止删除。Agents 携带
// 关联名单，handler 以 409 + data.agents 返回（issue #88）。
type ToolInUseError struct {
	ToolName string
	Agents   []string
}

func (e *ToolInUseError) Error() string {
	return fmt.Sprintf("Tool '%s' 仍被以下 Agent 挂载，请先解除关联：%s", e.ToolName, strings.Join(e.Agents, "、"))
}
