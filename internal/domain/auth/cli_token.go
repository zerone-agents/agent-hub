package auth

import "time"

// CLIToken is a long-lived opaque token for zhub CLI auth.
// Plaintext form: cli_<32hex> (4 + 32 characters). DB stores only SHA-256 hash, never plaintext.
type CLIToken struct {
	ID         uint64     `gorm:"primaryKey;autoIncrement"                                 json:"id"`
	UserID     string     `gorm:"column:user_id;type:varchar(128);index;not null"         json:"userId"`
	Name       string     `gorm:"type:varchar(128);not null"                               json:"name"`
	TokenHash  string     `gorm:"column:token_hash;type:varchar(64);uniqueIndex;not null" json:"-"`
	ExpiresAt  time.Time  `gorm:"column:expires_at;not null"                               json:"expiresAt"`
	CreatedAt  time.Time  `gorm:"column:created_at"                                        json:"createdAt"`
	LastUsedAt *time.Time `gorm:"column:last_used_at"                                      json:"lastUsedAt"`
}

// TableName overrides the default table name.
func (CLIToken) TableName() string { return "cli_tokens" }
