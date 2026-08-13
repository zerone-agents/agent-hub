package auth

import "time"

// RefreshToken is a long-lived opaque session token of the form rt_<hex>.
// The DB stores only the SHA-256 hash. Revocation is performed by deleting the
// row; rotation deletes the old row and inserts a fresh one.
type RefreshToken struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"                                 json:"id"`
	UserID    uint64    `gorm:"column:user_id;index;not null"                            json:"userId"`
	TokenHash string    `gorm:"column:token_hash;type:varchar(64);uniqueIndex;not null" json:"-"`
	ExpiresAt time.Time `gorm:"column:expires_at;not null"                               json:"expiresAt"`
	CreatedAt time.Time `gorm:"column:created_at"                                        json:"createdAt"`
}

// TableName overrides the default table name.
func (RefreshToken) TableName() string { return "refresh_tokens" }
