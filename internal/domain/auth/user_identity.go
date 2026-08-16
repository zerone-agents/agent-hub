package auth

import "time"

// UserIdentity is a shadow record linking an external (casdoor) account to
// a stable internal id, its tenant (casdoor organization), and a role
// snapshot for display/degradation. Tokens remain the authority for roles.
type UserIdentity struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"                    json:"id"`
	Provider    string    `gorm:"type:varchar(16);not null;uniqueIndex:idx_provider_external" json:"provider"`   // "casdoor"
	ExternalID  string    `gorm:"type:varchar(64);not null;uniqueIndex:idx_provider_external" json:"externalId"` // casdoor user Id
	TenantID    string    `gorm:"type:varchar(64);not null;index"            json:"tenantId"`
	Username    string    `gorm:"type:varchar(64)"                           json:"username"`
	DisplayName string    `gorm:"type:varchar(64)"                           json:"displayName"`
	Email       string    `gorm:"type:varchar(128)"                          json:"email"`
	Role        string    `gorm:"type:varchar(16)"                           json:"role"` // highest normalized role snapshot
	LastLoginAt time.Time `gorm:"column:last_login_at"                       json:"lastLoginAt"`
	CreatedAt   time.Time `gorm:"column:created_at"                          json:"createdAt"`
	UpdatedAt   time.Time `gorm:"column:updated_at"                          json:"updatedAt"`
}

// TableName overrides the default table name.
func (UserIdentity) TableName() string { return "user_identities" }
