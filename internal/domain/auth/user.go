package auth

import "time"

const (
	RoleAdmin      = "admin"
	RoleMaintainer = "maintainer"
	RoleMember     = "member"

	StatusActive   = "active"
	StatusDisabled = "disabled"
)

// IsValidRole reports whether r is one of the three built-in roles.
func IsValidRole(r string) bool {
	return r == RoleAdmin || r == RoleMaintainer || r == RoleMember
}

// User is a builtin local account. PasswordHash is bcrypt; never serialized
// out of the API (json:"-").
type User struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement"                          json:"id"`
	Username     string    `gorm:"type:varchar(32);uniqueIndex;not null"             json:"username"`
	PasswordHash string    `gorm:"column:password_hash;type:varchar(128);not null"   json:"-"`
	DisplayName  string    `gorm:"type:varchar(64)"                                  json:"displayName"`
	Email        string    `gorm:"type:varchar(128)"                                 json:"email"`
	Role         string    `gorm:"type:varchar(16);not null"                         json:"role"`
	Status       string    `gorm:"type:varchar(16);not null;default:active"          json:"status"`
	CreatedAt    time.Time `gorm:"column:created_at"                                 json:"createdAt"`
	UpdatedAt    time.Time `gorm:"column:updated_at"                                 json:"updatedAt"`
}

// TableName overrides the default table name.
func (User) TableName() string { return "users" }
