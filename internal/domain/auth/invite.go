package auth

import "time"

// Invite is a one-time registration link. TokenHash stores the SHA-256 of the
// plaintext inv_<hex> token; the plaintext is returned exactly once at creation
// and is never retrievable afterwards.
type Invite struct {
	ID        uint64     `gorm:"primaryKey;autoIncrement"                                 json:"id"`
	TokenHash string     `gorm:"column:token_hash;type:varchar(64);uniqueIndex;not null" json:"-"`
	Role      string     `gorm:"type:varchar(16);not null"                                json:"role"`
	Note      string     `gorm:"type:varchar(128)"                                        json:"note"`
	ExpiresAt time.Time  `gorm:"column:expires_at;not null"                               json:"expiresAt"`
	UsedAt    *time.Time `gorm:"column:used_at"                                           json:"usedAt"`
	CreatedBy uint64     `gorm:"column:created_by;not null"                               json:"createdBy"`
	CreatedAt time.Time  `gorm:"column:created_at"                                        json:"createdAt"`
}

// TableName overrides the default table name.
func (Invite) TableName() string { return "invites" }
