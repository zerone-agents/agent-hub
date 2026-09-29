package agent

import (
	"errors"
	"fmt"
	"strings"
)

// StableCode 稳定错误码命名类型（PR #204 评审建议）：构造 NewCodedValidationErrorf
// 时以码常量为唯一合法入参，裸字符串变量拼错在编译期即被拒绝（字面量仍可
// 隐式转换，配合 handler 侧钉住测试防漂移）。
type StableCode string

// ValidationError 标记用户面校验错误（请求参数/领域规则不合法）：
// HTTP 边界（respondAgentError）返回 400 + 原文中文文案；与之相对，
// 内部诊断（DB/加解密等基础设施包装）走 500 中性文案，完整错误链只在
// 服务端日志（issue #95 P2：handler 边界分流）。
type ValidationError struct {
	msg string
	// code 稳定错误码（issue #201 B 档）：非空时 HTTP 边界按码下发而非
	// 笼统的 invalid_agent_config；空串 = 未分类，回落粗码。
	code StableCode
	// params 前端 en 翻译的插值参数（值一律 string，不透传任意类型）。
	params map[string]string
}

func (e *ValidationError) Error() string { return e.msg }

// StableCode 返回稳定错误码；空串表示未分类（回落 invalid_agent_config）。
func (e *ValidationError) StableCode() StableCode { return e.code }

// Params 返回插值参数；nil 表示无参数。
func (e *ValidationError) Params() map[string]string { return e.params }

func NewValidationErrorf(format string, args ...any) error {
	return &ValidationError{msg: fmt.Sprintf(format, args...)}
}

// NewCodedValidationErrorf 构造携带稳定错误码与插值参数的校验错误
// （issue #201 B 档）：msg 仍为中文原文（zh 模式直出、信息不降级），
// code/params 供前端 en 模式按 apiErrors.<code> 键插值翻译。code 必须
// 注册于 internal/handler/errcodes.go；params 值只能是 string。
func NewCodedValidationErrorf(code StableCode, params map[string]string, format string, args ...any) error {
	return &ValidationError{msg: fmt.Sprintf(format, args...), code: code, params: params}
}

// 稳定错误码常量（issue #201 B 档 + PR #204 评审补充）：services 层构造
// NewCodedValidationErrorf 时引用；值必须与 internal/handler/errcodes.go
// 注册表及前端 apiErrors.* 键逐字一致（#149 P5 双写契约；两注册表由
// TestErrcodes_RegistryPinsDomainAgentCodes 钉死防漂移）。
const (
	CodeAgentNameRequired StableCode = "agent_name_required"
	CodeAgentNameTooLong  StableCode = "agent_name_too_long"
	CodeAgentNameInvalid  StableCode = "agent_name_invalid"
	CodeAgentNameExists   StableCode = "agent_name_exists"
	CodeAgentNotFound     StableCode = "agent_not_found"
	// CodeAgentReferenceNotFound 400 校验路径专用（DeleteAgent/
	// UpdateSubagents 引用的 Agent 不存在，带 name 插值）；与 404 sentinel
	// 的 CodeAgentNotFound 拆键，避免共用文案丢失插值（PR #204 评审）。
	CodeAgentReferenceNotFound StableCode = "agent_reference_not_found"
	CodeSubagentNotFound       StableCode = "subagent_not_found"
	CodeSubagentSelfReference  StableCode = "subagent_self_reference"
	CodeSystemPromptRequired   StableCode = "system_prompt_required"
	CodeConfigKeyRenamed       StableCode = "config_key_renamed"
	// 表单高频路径（PR #204 评审补充）
	CodeInvalidPermissionMode  StableCode = "invalid_permission_mode"
	CodeMaxTurnsNegative       StableCode = "max_turns_negative"
	CodeMaxTurnsTooLarge       StableCode = "max_turns_too_large"
	CodeProviderIdNotFound     StableCode = "provider_id_not_found"
	CodeModelSelectionNotFound StableCode = "model_selection_not_found"
	CodeModelNotFound          StableCode = "model_not_found"
	CodeModelTypeMismatch      StableCode = "model_type_mismatch"
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
