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

type CountDetail struct {
	Count int `json:"count"`
}

type AigcConfigDetail struct {
	ChangedFields []aigc.AigcConfigField `json:"changedFields"`
}
