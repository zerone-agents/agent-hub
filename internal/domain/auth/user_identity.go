package auth

import "time"

// UserIdentity 是 casdoor 模式下用户的租户成员记录（真实源）：身份映射 + 本地管理的角色 + 审批状态。
// Role 合法值仅 admin/maintainer/member，空 = 未分配（待审批）。
type UserIdentity struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"                    json:"id"`
	Provider    string    `gorm:"type:varchar(16);not null;uniqueIndex:idx_provider_external" json:"provider"`   // "casdoor"
	ExternalID  string    `gorm:"type:varchar(64);not null;uniqueIndex:idx_provider_external" json:"externalId"` // casdoor user Id
	TenantID    string    `gorm:"type:varchar(64);not null;index"            json:"tenantId"`
	Username    string    `gorm:"type:varchar(64)"                           json:"username"`
	DisplayName string    `gorm:"type:varchar(64)"                           json:"displayName"`
	Email       string    `gorm:"type:varchar(128)"                          json:"email"`
	Role        string    `gorm:"type:varchar(16)"                           json:"role"`   // 本地管理的角色（真实源）；空 = 未分配
	Status      string    `gorm:"type:varchar(16);not null;default:pending"  json:"status"` // 审批状态：pending/active
	LastLoginAt time.Time `gorm:"column:last_login_at"                       json:"lastLoginAt"`
	CreatedAt   time.Time `gorm:"column:created_at"                          json:"createdAt"`
	UpdatedAt   time.Time `gorm:"column:updated_at"                          json:"updatedAt"`
}

// TableName overrides the default table name.
func (UserIdentity) TableName() string { return "user_identities" }
