// H7.4 权限与隔离：extension_grants 与 extension_access_audit 模型。
//
// Grant 把扩展 manifest 声明的 permissions 落地为租户内的授权行：
// 安装时按 permissions[].mode（auto/approval，默认 auto）写入，
// auto 直接生效（status=active），approval 需管理员批准后生效
// （status=pending）。停用扩展只置 is_active=false（行保留，审计
// 可追溯）；卸载（非 purge）同样保留行；purge 才物理删除。
// Enforce 只认 status=active 且 is_active=true 且未撤销（revoked_at
// 为空）的行，因此撤销/停用即时生效。
package extension

import "time"

// 授权状态取值。
const (
	GrantStatusActive  = "active"  // auto 模式安装即生效
	GrantStatusPending = "pending" // approval 模式，待管理员批准
)

// Grant 是一条扩展授权记录（tenant × extension × permission × scope）。
type Grant struct {
	ID            uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID      string     `gorm:"type:varchar(64);not null;index:idx_extension_grants_tenant_ext,priority:1" json:"-"`
	ExtensionName string     `gorm:"type:varchar(253);not null;index:idx_extension_grants_tenant_ext,priority:2" json:"extensionName"`
	Permission    string     `gorm:"type:varchar(64);not null" json:"permission"`               // 权限类：manifest permission 白名单十类之一
	Scope         string     `gorm:"type:varchar(160);not null;default:''" json:"scope"`        // 命名空间（manifest 的 scope）
	Actions       string     `gorm:"type:text;not null" json:"actions"`                         // JSON 字符串数组
	Resource      string     `gorm:"type:varchar(253);not null;default:''" json:"resource"`     // 可选：具体资源（agent name / state namespace）
	Status        string     `gorm:"type:varchar(24);not null;default:'active'" json:"status"`  // active / pending
	Mode          string     `gorm:"type:varchar(24);not null;default:'auto'" json:"mode"`      // auto / approval（来自 manifest）
	IsActive      bool       `gorm:"not null;default:true;index" json:"isActive"`               // 停用扩展时置 false；Enforce 只认 true
	SourceVersion string     `gorm:"type:varchar(64);not null;default:''" json:"sourceVersion"` // 授权来源版本（升级/回滚时刷新）
	GrantedBy     string     `gorm:"type:varchar(160);not null;default:''" json:"grantedBy"`
	GrantedAt     time.Time  `json:"grantedAt"`
	RevokedAt     *time.Time `json:"revokedAt,omitempty"`
	LastSyncAt    time.Time  `json:"lastSyncAt"`
}

func (Grant) TableName() string { return "extension_grants" }

// AccessAudit 是一条扩展身份调用审计（Enforce 每次判定写一行，
// allowed 与 denied 都记）。
type AccessAudit struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID      string    `gorm:"type:varchar(64);not null;index:idx_extension_audit_tenant_ext,priority:1" json:"-"`
	ExtensionName string    `gorm:"type:varchar(253);not null;index:idx_extension_audit_tenant_ext,priority:2;index:idx_extension_audit_created" json:"extensionName"`
	Permission    string    `gorm:"type:varchar(64);not null" json:"permission"`
	Scope         string    `gorm:"type:varchar(160);not null;default:''" json:"scope"`
	Action        string    `gorm:"type:varchar(64);not null" json:"action"`
	Resource      string    `gorm:"type:varchar(253);not null;default:''" json:"resource"`
	Allowed       bool      `gorm:"not null" json:"allowed"`
	DeniedReason  string    `gorm:"type:varchar(253);not null;default:''" json:"deniedReason"`
	IP            string    `gorm:"type:varchar(64);not null;default:''" json:"ip"`
	CreatedAt     time.Time `gorm:"index:idx_extension_audit_created" json:"createdAt"`
}

func (AccessAudit) TableName() string { return "extension_access_audit" }
