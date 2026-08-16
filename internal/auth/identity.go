package auth

import (
	"errors"
	"time"

	authdom "control-panel/internal/domain/auth"

	"gorm.io/gorm"
)

// UpsertIdentity inserts or refreshes the user_identities shadow row for an
// external identity. Called on successful external (casdoor) logins.
func UpsertIdentity(db *gorm.DB, provider string, au *AuthUser) error {
	role := ""
	if len(au.Roles) > 0 {
		role = au.Roles[0]
	}
	now := time.Now()
	var row authdom.UserIdentity
	err := db.Where("provider = ? AND external_id = ?", provider, au.ID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&authdom.UserIdentity{
			Provider: provider, ExternalID: au.ID, TenantID: au.TenantID,
			Username: au.Username, DisplayName: au.DisplayName, Email: au.Email,
			Role: role, LastLoginAt: now,
		}).Error
	}
	if err != nil {
		return err
	}
	return db.Model(&row).Updates(map[string]any{
		"tenant_id": au.TenantID, "username": au.Username,
		"display_name": au.DisplayName, "email": au.Email,
		"role": role, "last_login_at": now,
	}).Error
}
