// ExtensionAuthzService 是 H7.4 权限与隔离的应用服务：
// 把扩展 manifest 声明的 permissions 落地为 extension_grants 授权行
// （mode=auto 安装即生效；mode=approval 写入 pending，管理员批准后
// 生效），提供实时 Enforce 判定（只认 status=active 且 is_active=true
// 且未撤销的授权，因此撤销/停用即时生效）、撤销、查询与调用审计
// （extension_access_audit，allowed/denied 都记）。
// 全部按租户隔离；非扩展身份请求不经过本服务，零影响。
package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"control-panel/internal/domain/extension"
	"gorm.io/gorm"
)

// ExtensionAuthzService 依赖 GORM，可直接接 cmd/server 传入的数据库句柄。
type ExtensionAuthzService struct{ db *gorm.DB }

func NewExtensionAuthzService(db *gorm.DB) *ExtensionAuthzService {
	return &ExtensionAuthzService{db: db}
}

// manifestPermissionDecl 是权限声明的扩展解析形态：在严格校验通过的
// manifest 之上补充读取可选的 mode（auto/approval，默认 auto）与
// resource（可选具体资源）；未知字段不影响 H7.0 严格校验。
type manifestPermissionDecl struct {
	Permission string   `json:"permission"`
	Scope      string   `json:"scope"`
	Actions    []string `json:"actions"`
	Mode       string   `json:"mode"`
	Resource   string   `json:"resource"`
}

// parsePermissionDecls 从已注册版本的 manifest JSON 中解析权限声明
// （含可选 mode/resource）。manifest 无 permissions 段时返回空。
func parsePermissionDecls(manifestRaw string) ([]manifestPermissionDecl, error) {
	var probe struct {
		Permissions []manifestPermissionDecl `json:"permissions"`
	}
	if err := json.Unmarshal([]byte(manifestRaw), &probe); err != nil {
		return nil, fmt.Errorf("解析 manifest 权限声明失败: %w", err)
	}
	for i := range probe.Permissions {
		mode := strings.TrimSpace(probe.Permissions[i].Mode)
		if mode == "" {
			mode = "auto"
		}
		if mode != "auto" && mode != "approval" {
			return nil, fmt.Errorf("permissions[%d].mode 必须是 auto/approval，当前为 %q", i, mode)
		}
		probe.Permissions[i].Mode = mode
		probe.Permissions[i].Permission = strings.TrimSpace(probe.Permissions[i].Permission)
		probe.Permissions[i].Scope = strings.TrimSpace(probe.Permissions[i].Scope)
		probe.Permissions[i].Resource = strings.TrimSpace(probe.Permissions[i].Resource)
	}
	return probe.Permissions, nil
}

func grantKey(permission, scope, resource string) string {
	return permission + "" + scope + "" + resource
}

// SyncGrantsFromManifest 按 manifest 权限声明同步授权行（安装/升级后调用）：
// 声明内权限 upsert（存在则刷新 actions/status/is_active，不存在则新建，
// mode=auto → status=active，mode=approval → status=pending）；已不在声明中
// 的授权行做软撤销（revoked_at 置位、is_active=false），保留审计。
// 幂等：重复同步同版本不会产生重复行。
func (s *ExtensionAuthzService) SyncGrantsFromManifest(tenantID, extensionName, manifestRaw, version, grantedBy string) error {
	tenantID = normalizedTenant(tenantID)
	extensionName = strings.TrimSpace(extensionName)
	decls, err := parsePermissionDecls(manifestRaw)
	if err != nil {
		return err
	}
	var existing []extension.Grant
	if err := s.db.Where("tenant_id=? AND extension_name=?", tenantID, extensionName).Find(&existing).Error; err != nil {
		return err
	}
	byKey := map[string]*extension.Grant{}
	for i := range existing {
		g := &existing[i]
		if g.RevokedAt == nil {
			byKey[grantKey(g.Permission, g.Scope, g.Resource)] = g
		}
	}
	now := time.Now()
	seen := map[string]bool{}
	for _, decl := range decls {
		if decl.Permission == "" {
			continue
		}
		key := grantKey(decl.Permission, decl.Scope, decl.Resource)
		seen[key] = true
		actionsJSON, err := json.Marshal(decl.Actions)
		if err != nil {
			return err
		}
		status := extension.GrantStatusActive
		if decl.Mode == "approval" {
			status = extension.GrantStatusPending
		}
		if g, ok := byKey[key]; ok {
			updates := map[string]any{
				"actions":        string(actionsJSON),
				"mode":           decl.Mode,
				"is_active":      true,
				"source_version": version,
				"last_sync_at":   now,
			}
			// pending 行保持 pending（等批准）；active 行保持 active
			if g.Status != extension.GrantStatusPending {
				updates["status"] = status
			}
			if err := s.db.Model(&extension.Grant{}).Where("id=?", g.ID).Updates(updates).Error; err != nil {
				return err
			}
			continue
		}
		grant := extension.Grant{
			TenantID:      tenantID,
			ExtensionName: extensionName,
			Permission:    decl.Permission,
			Scope:         decl.Scope,
			Actions:       string(actionsJSON),
			Resource:      decl.Resource,
			Status:        status,
			Mode:          decl.Mode,
			IsActive:      true,
			SourceVersion: version,
			GrantedBy:     strings.TrimSpace(grantedBy),
			GrantedAt:     now,
			LastSyncAt:    now,
		}
		if err := s.db.Create(&grant).Error; err != nil {
			return fmt.Errorf("写入扩展授权失败: %w", err)
		}
	}
	// 声明中已移除的授权：软撤销（保留行与审计）
	for key, g := range byKey {
		if seen[key] {
			continue
		}
		if err := s.db.Model(&extension.Grant{}).Where("id=?", g.ID).Updates(map[string]any{
			"is_active":  false,
			"revoked_at": now,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

// SetGrantsActive 启用/停用扩展时联动授权生效状态（停用不删行，审计保留）。
func (s *ExtensionAuthzService) SetGrantsActive(tenantID, extensionName string, active bool) error {
	tenantID = normalizedTenant(tenantID)
	return s.db.Model(&extension.Grant{}).
		Where("tenant_id=? AND extension_name=? AND revoked_at IS NULL", tenantID, extensionName).
		Update("is_active", active).Error
}

// DeleteGrantsForExtension 仅 purge 卸载时调用：物理删除授权与审计行。
func (s *ExtensionAuthzService) DeleteGrantsForExtension(tenantID, extensionName string) error {
	tenantID = normalizedTenant(tenantID)
	if err := s.db.Where("tenant_id=? AND extension_name=?", tenantID, extensionName).Delete(&extension.Grant{}).Error; err != nil {
		return err
	}
	return s.db.Where("tenant_id=? AND extension_name=?", tenantID, extensionName).Delete(&extension.AccessAudit{}).Error
}

// Enforce 判定扩展在某权限类/动作/资源上是否有有效授权，并写审计行。
// 实时查询数据库：撤销/停用/批准立即反映。返回是否放行；denied 时
// 可通过 LastDenyReason 读取原因。
func (s *ExtensionAuthzService) Enforce(tenantID, extensionName, permission, action, resource, ip string) bool {
	tenantID = normalizedTenant(tenantID)
	extensionName = strings.TrimSpace(extensionName)
	permission = strings.TrimSpace(permission)
	action = strings.TrimSpace(action)
	resource = strings.TrimSpace(resource)

	allowed, reason := s.check(tenantID, extensionName, permission, action, resource)
	audit := extension.AccessAudit{
		TenantID:      tenantID,
		ExtensionName: extensionName,
		Permission:    permission,
		Scope:         "",
		Action:        action,
		Resource:      resource,
		Allowed:       allowed,
		DeniedReason:  reason,
		IP:            ip,
		CreatedAt:     time.Now(),
	}
	// 审计写入失败不阻断判定结果（判定已基于授权行完成）
	_ = s.db.Create(&audit).Error
	return allowed
}

// check 是纯判定逻辑（不写审计），供 Enforce 与测试复用。
func (s *ExtensionAuthzService) check(tenantID, extensionName, permission, action, resource string) (bool, string) {
	var grants []extension.Grant
	err := s.db.Where("tenant_id=? AND extension_name=? AND permission=? AND is_active=? AND status=? AND revoked_at IS NULL",
		tenantID, extensionName, permission, true, extension.GrantStatusActive).Find(&grants).Error
	if err != nil {
		return false, "授权查询失败: " + err.Error()
	}
	for _, g := range grants {
		// grant 声明了具体 resource 时，请求必须精确匹配（请求留空也不放行）
		if g.Resource != "" && g.Resource != resource {
			continue
		}
		var actions []string
		if json.Unmarshal([]byte(g.Actions), &actions) != nil {
			continue
		}
		for _, a := range actions {
			if a == action || a == "*" {
				return true, ""
			}
		}
	}
	if len(grants) == 0 {
		return false, "扩展未被授予该权限"
	}
	return false, "扩展未被授予该权限: " + permission + "/" + action
}

// RevokeGrant 撤销单条授权：revoked_at 置位 + is_active=false，Enforce 立即失效。
func (s *ExtensionAuthzService) RevokeGrant(tenantID string, extID uint64, grantID uint64) error {
	tenantID = normalizedTenant(tenantID)
	ext, err := s.loadExtension(tenantID, extID)
	if err != nil {
		return err
	}
	res := s.db.Model(&extension.Grant{}).
		Where("id=? AND tenant_id=? AND extension_name=? AND revoked_at IS NULL", grantID, tenantID, ext.Name).
		Updates(map[string]any{"revoked_at": time.Now(), "is_active": false})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return lifecycleErrorf(404, "授权不存在或已撤销")
	}
	return nil
}

// ApproveGrant 批准 pending 授权（manifest mode=approval 时）。
func (s *ExtensionAuthzService) ApproveGrant(tenantID string, extID uint64, grantID uint64, approver string) error {
	tenantID = normalizedTenant(tenantID)
	ext, err := s.loadExtension(tenantID, extID)
	if err != nil {
		return err
	}
	res := s.db.Model(&extension.Grant{}).
		Where("id=? AND tenant_id=? AND extension_name=? AND status=?", grantID, tenantID, ext.Name, extension.GrantStatusPending).
		Updates(map[string]any{"status": extension.GrantStatusActive, "granted_by": strings.TrimSpace(approver)})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return lifecycleErrorf(404, "待批准授权不存在")
	}
	return nil
}

func (s *ExtensionAuthzService) loadExtension(tenantID string, id uint64) (*extension.Extension, error) {
	var ext extension.Extension
	err := s.db.Where("tenant_id=? AND id=?", tenantID, id).First(&ext).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, lifecycleErrorf(404, "扩展不存在")
	}
	if err != nil {
		return nil, err
	}
	return &ext, nil
}

// ListGrants 返回租户内某扩展的全部授权行（含已撤销，供审计展示）。
func (s *ExtensionAuthzService) ListGrants(tenantID string, extID uint64) ([]extension.Grant, error) {
	tenantID = normalizedTenant(tenantID)
	ext, err := s.loadExtension(tenantID, extID)
	if err != nil {
		return nil, err
	}
	var grants []extension.Grant
	if err := s.db.Where("tenant_id=? AND extension_name=?", tenantID, ext.Name).Order("id").Find(&grants).Error; err != nil {
		return nil, err
	}
	return grants, nil
}

// AuditFilter 是审计分页过滤条件。
type AuditFilter struct {
	Page     int
	PageSize int
	Allowed  *bool // nil=全部
}

// AuditPage 是审计分页结果。
type AuditPage struct {
	Items    []extension.AccessAudit `json:"items"`
	Total    int64                   `json:"total"`
	Page     int                     `json:"page"`
	PageSize int                     `json:"pageSize"`
}

// ListAudit 返回租户内某扩展的调用审计（时间倒序分页）。
func (s *ExtensionAuthzService) ListAudit(tenantID string, extID uint64, filter AuditFilter) (*AuditPage, error) {
	tenantID = normalizedTenant(tenantID)
	ext, err := s.loadExtension(tenantID, extID)
	if err != nil {
		return nil, err
	}
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	base := s.db.Model(&extension.AccessAudit{}).Where("tenant_id=? AND extension_name=?", tenantID, ext.Name)
	if filter.Allowed != nil {
		base = base.Where("allowed=?", *filter.Allowed)
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, err
	}
	var items []extension.AccessAudit
	if err := base.Order("created_at DESC, id DESC").
		Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).Find(&items).Error; err != nil {
		return nil, err
	}
	if items == nil {
		items = []extension.AccessAudit{}
	}
	return &AuditPage{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}
