package audit

import "time"

type Category string
type Action string
type Status string
type TargetType string

const (
	CatAuth     Category = "auth"
	CatUser     Category = "user"
	CatInvite   Category = "invite"
	CatProvider Category = "provider"
	CatAgent    Category = "agent"
	CatToken    Category = "token"
	CatAigc     Category = "aigc"
)

const (
	ActionLogin          Action = "auth.login"
	ActionLogout         Action = "auth.logout"
	ActionSetup          Action = "auth.setup"
	ActionPasswordChange Action = "auth.password_change"

	ActionUpdateRole    Action = "user.update_role"
	ActionUpdateStatus  Action = "user.update_status"
	ActionResetPassword Action = "user.reset_password"
	ActionLoginURL      Action = "user.login_url"

	ActionInviteCreate Action = "invite.create"
	ActionInviteRevoke Action = "invite.revoke"

	ActionRevealKey    Action = "provider.reveal_key"
	ActionSyncMultirag Action = "provider.sync_multirag"

	ActionDeploy   Action = "agent.deploy"
	ActionStop     Action = "agent.stop"
	ActionStart    Action = "agent.start"
	ActionUndeploy Action = "agent.undeploy"
	ActionDelete   Action = "agent.delete"

	ActionCliTokenIssue  Action = "cli_token.issue"
	ActionCliTokenRevoke Action = "cli_token.revoke"

	ActionAigcSave      Action = "aigc.save"
	ActionAigcDelete    Action = "aigc.delete"
	ActionAigcRotateKey Action = "aigc.rotate_key"
)

const (
	StatusSuccess Status = "success"
	StatusFailure Status = "failure"
	StatusPartial Status = "partial"
)

const (
	TargetUser       TargetType = "user"
	TargetInvite     TargetType = "invite"
	TargetProvider   TargetType = "provider"
	TargetAgent      TargetType = "agent"
	TargetToken      TargetType = "token"
	TargetAigcConfig TargetType = "aigc_config"
	TargetSystem     TargetType = "system"
)

// Log 是 audit_logs 表模型（spec §4）。ID/Detail 的 json tag 为 "-"：
// API 侧经 Querier DTO 输出（id 十进制字符串、detail JSON 对象），
// 模型不直接参与 API 序列化。
type Log struct {
	ID         uint64     `gorm:"primaryKey;autoIncrement;index:idx_audit_tenant_created,priority:3" json:"-"`
	TenantID   string     `gorm:"size:64;index:idx_audit_tenant_created,priority:1" json:"tenantId"`
	UserID     string     `gorm:"size:64" json:"userId"`
	UserName   string     `gorm:"size:64" json:"userName"`
	Category   Category   `gorm:"size:32;index" json:"category"`
	Action     Action     `gorm:"size:64;index" json:"action"`
	TargetType TargetType `gorm:"size:32" json:"targetType"`
	TargetID   string     `gorm:"size:64" json:"targetId"`
	TargetName string     `gorm:"size:128" json:"targetName"`
	Status     Status     `gorm:"size:16" json:"status"`
	Detail     string     `gorm:"type:text" json:"-"`
	RemoteIP   string     `gorm:"size:45" json:"remoteIp"`
	UserAgent  string     `gorm:"size:256" json:"userAgent"`
	CreatedAt  time.Time  `gorm:"type:datetime(6);index:idx_audit_tenant_created,priority:2" json:"createdAt"`
}

// TableName 显式映射 audit_logs（spec §4；GORM 默认会把 Log 映射成 logs）。
func (Log) TableName() string { return "audit_logs" }
