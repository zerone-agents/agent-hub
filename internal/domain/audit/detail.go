package audit

import "control-panel/internal/domain/aigc"

// Detail 强类型白名单（spec §5.4）：字段即闭集，密码/Key/token/链接/邀请码
// 片段在类型层面不存在。wire key 显式 camelCase。
type ChangeDetail struct {
	Field string `json:"field"`
	From  string `json:"from"`
	To    string `json:"to"`
}

const (
	ReasonInvalidCredentials  = "invalid_credentials"
	ReasonTokenIssuanceFailed = "token_issuance_failed"
	// ReasonInvalidRequest：casdoor callback 前期失败（参数缺失 / state 无效）——
	// 既非凭校验失败也非签发失败的第三类（PR #150 审查 P2，spec §5.6 阶段表）。
	ReasonInvalidRequest = "invalid_request"
)

type LoginDetail struct {
	Username string `json:"username"`
	Org      string `json:"org"`
	Reason   string `json:"reason"` // 成功为空；失败为 reason 常量
}

type InviteDetail struct {
	Role          string `json:"role"`
	ExpiresInDays int    `json:"expiresInDays"`
}

type AigcConfigDetail struct {
	ChangedFields []aigc.AigcConfigField `json:"changedFields"`
}

// BindingKind 是 agent 挂载关系变更的资源类型闭集（#210 审查建议：
// 对齐 AigcConfigField 的强类型先例，裸字符串退化在类型层面避免）。
type BindingKind string

const (
	BindingKindSkill BindingKind = "skill"
	BindingKindTool  BindingKind = "tool"
	BindingKindMcp   BindingKind = "mcp"
)

// BindingDetail：agent 挂载关系变更（issue #210）。Kind 为资源类型
// （skill / tool / mcp），Names 为本次写入后的完整生效名单。
type BindingDetail struct {
	Kind  BindingKind `json:"kind"`
	Names []string    `json:"names"`
}

// AgentUpdateDetail：agent.update 的变更字段名（仅字段名，不含值——
// 字段值可能含敏感配置，spec §5.4 白名单原则）。
type AgentUpdateDetail struct {
	Fields []string `json:"fields"`
}
