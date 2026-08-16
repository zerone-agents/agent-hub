package auth

import (
	"errors"
	"time"

	authdom "control-panel/internal/domain/auth"

	"gorm.io/gorm"
)

// MembershipStore 是 user_identities 的持久化接口（Task 4/5 注入 provider）。
type MembershipStore interface {
	// FindByExternalID 返回 (nil, nil) 表示无记录。
	FindByExternalID(provider, externalID string) (*authdom.UserIdentity, error)
	// ApplyDecision 按合成结果建/更新记录；OpNone 时仅刷新快照与 lastLoginAt。
	// 快照字段（username/displayName/email/tenantID）随每次调用刷新。
	ApplyDecision(provider string, au *AuthUser, d MembershipDecision) error
}

// gormMembershipStore 是 MembershipStore 的 gorm 实现。
type gormMembershipStore struct {
	db *gorm.DB
}

// NewMembershipStore 返回基于 gorm 的 MembershipStore。
func NewMembershipStore(db *gorm.DB) MembershipStore {
	return &gormMembershipStore{db: db}
}

// FindByExternalID 按 (provider, externalID) 查成员记录；无记录返回 (nil, nil)。
func (s *gormMembershipStore) FindByExternalID(provider, externalID string) (*authdom.UserIdentity, error) {
	var row authdom.UserIdentity
	err := s.db.Where("provider = ? AND external_id = ?", provider, externalID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// ApplyDecision 按合成结果落库：
//   - 无记录且 Op=OpCreate → 建记录（Role/Status 取 decision，快照取 au）；
//   - 有记录 → 始终刷新快照与 last_login_at；仅 Op=OpUpdate 时额外更新 role/status；
//   - Op=None 绝不动 role/status（角色只能经用户管理或合成规则改变，不受登录路径影响）。
func (s *gormMembershipStore) ApplyDecision(provider string, au *AuthUser, d MembershipDecision) error {
	now := time.Now()
	rec, err := s.FindByExternalID(provider, au.ID)
	if err != nil {
		return err
	}
	if rec == nil {
		if d.Op != OpCreate {
			// 无记录但合成结果不要求建记录：视为无操作（防御性分支，正常不会走到）。
			return nil
		}
		return s.db.Create(&authdom.UserIdentity{
			Provider: provider, ExternalID: au.ID, TenantID: au.TenantID,
			Username: au.Username, DisplayName: au.DisplayName, Email: au.Email,
			Role: d.Role, Status: d.Status, LastLoginAt: now,
		}).Error
	}
	updates := map[string]any{
		"tenant_id":     au.TenantID,
		"username":      au.Username,
		"display_name":  au.DisplayName,
		"email":         au.Email,
		"last_login_at": now,
	}
	if d.Op == OpUpdate {
		updates["role"] = d.Role
		updates["status"] = d.Status
	}
	return s.db.Model(rec).Updates(updates).Error
}
