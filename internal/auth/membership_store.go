package auth

import (
	"errors"
	"time"

	authdom "control-panel/internal/domain/auth"

	"gorm.io/gorm"
)

// MembershipStore 是 user_identities 的持久化接口（Task 4/5 注入 provider，
// Task 6 追加用户管理路径的 ListByTenant/SetRole）。
type MembershipStore interface {
	// FindByExternalID 返回 (nil, nil) 表示无记录。
	FindByExternalID(provider, externalID string) (*authdom.UserIdentity, error)
	// ListByTenant 返回租户下全部成员记录（按 id 升序，列表顺序稳定）。
	ListByTenant(tenantID string) ([]authdom.UserIdentity, error)
	// ApplyDecision 按合成结果建/更新记录；OpNone 时仅刷新快照与 lastLoginAt。
	// 快照字段（username/displayName/email/tenantID）随每次调用刷新。
	ApplyDecision(provider string, au *AuthUser, d MembershipDecision) error
	// SetRole 直接更新成员的 role/status（用户管理路径专用，绕过合成规则）。
	// 调用方应先经 FindByExternalID 确认记录存在；记录不存在时返回错误。
	SetRole(provider, externalID, role, status string) error
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

// ListByTenant 返回租户下全部成员记录，按 id 升序保证列表顺序稳定。
func (s *gormMembershipStore) ListByTenant(tenantID string) ([]authdom.UserIdentity, error) {
	var rows []authdom.UserIdentity
	if err := s.db.Where("tenant_id = ?", tenantID).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// SetRole 直接更新成员的 role/status 两列（用户管理路径，绕过合成规则）；
// 快照字段与 last_login_at 不受影响。记录不存在（如并发下被删）返回
// gorm.ErrRecordNotFound，不静默成功。
func (s *gormMembershipStore) SetRole(provider, externalID, role, status string) error {
	res := s.db.Model(&authdom.UserIdentity{}).
		Where("provider = ? AND external_id = ?", provider, externalID).
		Updates(map[string]any{"role": role, "status": status})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
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
