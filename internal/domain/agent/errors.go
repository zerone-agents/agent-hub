package agent

import (
	"errors"
	"fmt"
	"strings"
)

// ErrAgentNotFound Agent 行不存在：service 层依据 gorm.ErrRecordNotFound 按
// fmt.Errorf("%w: %s", ErrAgentNotFound, name) 包装，handler 用 errors.Is 映射
// 404（对齐 ErrToolNotFound；DB 故障走英文诊断包装 → 500 桶，绝不伪装 not-found）。
var ErrAgentNotFound = errors.New("Agent 不存在")

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
	// ErrInvalidToolName 工具名校验失败（含 deployer 契约拒绝的 "."/".."）。
	// ValidateToolName 的所有拒绝路径都包装本 sentinel，handler 据此映射 400
	// （expert review round 3：校验失败与基础设施故障分流）。
	ErrInvalidToolName = errors.New("Tool 标识无效")
)

// ToolInUseError 删除保护：仍被 Agent 关联的自定义工具禁止删除。Agents 携带
// 关联名单，handler 以 409 + data.agents 返回（issue #88）。Foreign 表示
// 他租户也挂载该工具（issue #123 收敛：409 载荷绝不透出他租户 Agent 名，
// 仅中性事实，对齐 DatasetInUseError review P1）。
type ToolInUseError struct {
	ToolName string
	Agents   []string
	Foreign  bool
}

func (e *ToolInUseError) Error() string {
	switch {
	case len(e.Agents) > 0 && e.Foreign:
		return fmt.Sprintf("Tool '%s' 仍被以下 Agent 挂载：%s（另被其他租户使用），请先解除关联", e.ToolName, strings.Join(e.Agents, "、"))
	case len(e.Agents) > 0:
		return fmt.Sprintf("Tool '%s' 仍被以下 Agent 挂载：%s，请先解除关联", e.ToolName, strings.Join(e.Agents, "、"))
	default:
		return fmt.Sprintf("Tool '%s' 仍被其他租户挂载，请先解除关联", e.ToolName)
	}
}

// SkillInUseError 删除保护：仍被 Agent 绑定的技能禁止删除（issue #123）。
// Agents 只含当前请求租户的名单；Foreign 表示他租户仍绑定（仅中性事实，
// 不携带任何他租户身份）。handler 以 409 + data{agents, foreign} 返回。
type SkillInUseError struct {
	SkillName string
	Agents    []string
	Foreign   bool
}

func (e *SkillInUseError) Error() string {
	switch {
	case len(e.Agents) > 0 && e.Foreign:
		return fmt.Sprintf("技能 '%s' 仍被 Agent 使用：%s（另被其他租户使用），请先解除绑定", e.SkillName, strings.Join(e.Agents, "、"))
	case len(e.Agents) > 0:
		return fmt.Sprintf("技能 '%s' 仍被 Agent 使用：%s，请先解除绑定", e.SkillName, strings.Join(e.Agents, "、"))
	default:
		return fmt.Sprintf("技能 '%s' 仍被其他租户使用，请先解除绑定", e.SkillName)
	}
}

// McpInUseError 删除保护：仍被 Agent 绑定的 MCP 服务器禁止删除（issue #123）。
// Agents 只含当前请求租户的名单；Foreign 表示他租户仍绑定（仅中性事实，
// 不携带任何他租户身份）。handler 以 409 + data{agents, foreign} 返回。
type McpInUseError struct {
	McpName string
	Agents  []string
	Foreign bool
}

func (e *McpInUseError) Error() string {
	switch {
	case len(e.Agents) > 0 && e.Foreign:
		return fmt.Sprintf("MCP '%s' 仍被 Agent 使用：%s（另被其他租户使用），请先解除绑定", e.McpName, strings.Join(e.Agents, "、"))
	case len(e.Agents) > 0:
		return fmt.Sprintf("MCP '%s' 仍被 Agent 使用：%s，请先解除绑定", e.McpName, strings.Join(e.Agents, "、"))
	default:
		return fmt.Sprintf("MCP '%s' 仍被其他租户使用，请先解除绑定", e.McpName)
	}
}

// DatasetInUseItem/DatasetInUseError 删除保护：仍被 Agent 绑定的知识库禁止
// 删除（issue #122，模式对齐 ToolInUseError #88）。多库批量删除时逐库携带
// 绑定 Agent 名单；dataset 用裸 ID——元数据在远端 multirag，错误路径不做
// 上游反查。handler 以 409 + data.datasets 返回。
// Agents 只含当前请求租户的名单；Foreign 表示他租户也绑定该库（review
// P1：防护跨租户防误删，但 409 载荷绝不透出他租户 Agent 名，仅中性事实）。
type DatasetInUseItem struct {
	ID      string
	Agents  []string
	Foreign bool
}

type DatasetInUseError struct {
	Datasets []DatasetInUseItem
}

func (e *DatasetInUseError) Error() string {
	parts := make([]string, 0, len(e.Datasets))
	for _, d := range e.Datasets {
		switch {
		case len(d.Agents) > 0 && d.Foreign:
			parts = append(parts, fmt.Sprintf("%s（%s；另被其他租户使用）", d.ID, strings.Join(d.Agents, "、")))
		case len(d.Agents) > 0:
			parts = append(parts, fmt.Sprintf("%s（%s）", d.ID, strings.Join(d.Agents, "、")))
		default:
			parts = append(parts, fmt.Sprintf("%s（仍被其他租户使用）", d.ID))
		}
	}
	return "知识库仍被 Agent 绑定，请先解除绑定：" + strings.Join(parts, "；")
}
