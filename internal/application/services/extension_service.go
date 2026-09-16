// ExtensionService 是 H7 扩展注册中心的应用服务：
// 注册扩展与版本（严格校验 + 内容哈希幂等）、按租户隔离的列表/详情查询、
// 以及 GetEnabled 语义查询。H6 能力包走 CapabilityRegistryService 不动，
// 本服务与之并存（见 H7 兼容层决策：最小侵入，H7.1 再统一生命周期）。
package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"control-panel/internal/domain/extension"
	"control-panel/internal/extensionmanifest"
	"gorm.io/gorm"
)

type ExtensionService struct{ db *gorm.DB }

func NewExtensionService(db *gorm.DB) *ExtensionService {
	return &ExtensionService{db: db}
}

type RegisterExtensionInput struct {
	Manifest  json.RawMessage // 严格模式 manifest JSON
	Source    string          // seed/registry/upload，默认 upload
	Changelog string
	CreatedBy string
	// Signature/PublicKey 可选：两者同时提供时走 ed25519 验签（H7.6），
	// 验签通过才把版本注册进 registry 并把公钥指纹记入 signed_by；
	// 任一缺失且另一非空则拒绝。均为 base64（或 hex）编码。
	Signature string
	PublicKey string
}

// RegisterResult 携带注册结果；AlreadyExisted 为 true 表示命中幂等
// （同租户同名同版本同内容哈希，直接返回既有版本，不报错）。
type RegisterResult struct {
	Extension      *extension.Extension
	Version        *extension.Version
	AlreadyExisted bool
}

// Register 注册扩展与版本。同 (tenant, name) 的扩展已存在则复用；
// 同 (extension, version, content_hash) 重复注册幂等返回；同版本不同内容报错。
func (s *ExtensionService) Register(tenantID string, in RegisterExtensionInput) (*RegisterResult, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		tenantID = "default"
	}
	manifest, errs := extensionmanifest.ValidateExtensionManifest(in.Manifest)
	if len(errs) > 0 {
		return nil, fmt.Errorf("扩展 manifest 校验失败：%s", strings.Join(errs, "；"))
	}
	// H7.6 验签：signature/public_key 成对出现才校验；验签失败直接拒绝。
	var signedBy string
	if strings.TrimSpace(in.Signature) != "" || strings.TrimSpace(in.PublicKey) != "" {
		fingerprint, err := extension.VerifyExtension(string(in.Manifest), in.Signature, in.PublicKey)
		if err != nil {
			return nil, err
		}
		signedBy = fingerprint
	}
	hash, err := extensionmanifest.CanonicalManifestHash(in.Manifest)
	if err != nil {
		return nil, err
	}
	source := strings.TrimSpace(in.Source)
	if source == "" {
		source = extension.SourceUpload
	}
	switch source {
	case extension.SourceSeed, extension.SourceRegistry, extension.SourceUpload:
	default:
		return nil, fmt.Errorf("source 必须是 seed/registry/upload 之一，当前为 %q", source)
	}

	result := &RegisterResult{}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var ext extension.Extension
		err := tx.Where("tenant_id=? AND name=?", tenantID, manifest.Name).First(&ext).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			ext = extension.Extension{
				TenantID:    tenantID,
				Name:        manifest.Name,
				DisplayName: manifest.DisplayName,
				Description: manifest.Description,
				Icon:        manifest.Icon,
				Source:      source,
				Status:      extension.StatusActive,
			}
			if err := tx.Create(&ext).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			// 已注册：展示字段允许随新版本 manifest 更新
			updates := map[string]any{}
			if ext.DisplayName != manifest.DisplayName {
				updates["display_name"] = manifest.DisplayName
			}
			if ext.Description != manifest.Description {
				updates["description"] = manifest.Description
			}
			if ext.Icon != manifest.Icon {
				updates["icon"] = manifest.Icon
			}
			if len(updates) > 0 {
				if err := tx.Model(&extension.Extension{}).Where("id=?", ext.ID).Updates(updates).Error; err != nil {
					return err
				}
			}
		}

		var ver extension.Version
		err = tx.Where("extension_id=? AND version=?", ext.ID, manifest.Version).First(&ver).Error
		switch {
		case err == nil:
			if ver.ContentHash != hash {
				return fmt.Errorf("扩展 %s 的版本 %s 已存在且内容不一致（内容寻址冲突）", manifest.Name, manifest.Version)
			}
			result.AlreadyExisted = true
			// 幂等路径补记签名指纹（历史未签名版本升级签名时生效）
			if signedBy != "" && ver.SignedBy == "" {
				if err := tx.Model(&extension.Version{}).Where("id=?", ver.ID).Update("signed_by", signedBy).Error; err != nil {
					return err
				}
				ver.SignedBy = signedBy
			}
			result.Version = &ver
		case errors.Is(err, gorm.ErrRecordNotFound):
			ver = extension.Version{
				ExtensionID: ext.ID,
				Version:     manifest.Version,
				Manifest:    string(in.Manifest),
				ContentHash: hash,
				Changelog:   strings.TrimSpace(in.Changelog),
				CreatedBy:   strings.TrimSpace(in.CreatedBy),
				SignedBy:    signedBy,
			}
			if err := tx.Create(&ver).Error; err != nil {
				return err
			}
			result.Version = &ver
		default:
			return err
		}
		result.Extension = &ext
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

type ExtensionListFilter struct {
	Status   string
	Source   string
	Page     int // 从 1 开始
	PageSize int // 默认 20，上限 100
}

type ExtensionListItem struct {
	extension.Extension
	VersionCount  int64  `json:"versionCount"`
	LatestVersion string `json:"latestVersion"`
	// H7.1：当前租户安装状态（installed=false 时后两者为空）
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installedVersion"`
	InstalledStatus  string `json:"installedStatus"`
}

type ExtensionListPage struct {
	Items    []ExtensionListItem `json:"items"`
	Total    int64               `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"pageSize"`
}

// List 按租户隔离列出扩展，支持 status/source 过滤与分页。
func (s *ExtensionService) List(tenantID string, filter ExtensionListFilter) (*ExtensionListPage, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		tenantID = "default"
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

	base := s.db.Model(&extension.Extension{}).Where("tenant_id=?", tenantID)
	if filter.Status != "" {
		base = base.Where("status=?", filter.Status)
	}
	if filter.Source != "" {
		base = base.Where("source=?", filter.Source)
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, err
	}

	var exts []extension.Extension
	if err := base.Order("name").Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).Find(&exts).Error; err != nil {
		return nil, err
	}
	items := make([]ExtensionListItem, 0, len(exts))
	for _, ext := range exts {
		var count int64
		var latest extension.Version
		if err := s.db.Model(&extension.Version{}).Where("extension_id=?", ext.ID).Count(&count).Error; err != nil {
			return nil, err
		}
		_ = s.db.Where("extension_id=?", ext.ID).Order("created_at DESC").First(&latest).Error
		item := ExtensionListItem{Extension: ext, VersionCount: count, LatestVersion: latest.Version}
		// H7.1：附带当前租户安装状态
		var inst extension.Install
		if err := s.db.Where("tenant_id=? AND extension_id=?", tenantID, ext.ID).First(&inst).Error; err == nil {
			item.Installed = true
			item.InstalledVersion = inst.Version
			item.InstalledStatus = inst.Status
		}
		items = append(items, item)
	}
	return &ExtensionListPage{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

// VersionManifestSummary 是详情页展示的单个版本 manifest 摘要。
type VersionManifestSummary struct {
	DisplayName          string   `json:"displayName"`
	Description          string   `json:"description"`
	Slots                []string `json:"slots,omitempty"`
	StateSchemaCount     int      `json:"stateSchemaCount"`
	EventCount           int      `json:"eventCount"`
	ToolCount            int      `json:"toolCount"`
	RelationCount        int      `json:"relationCount"`
	PromptInjectionCount int      `json:"promptInjectionCount"`
}

// PermissionSummary 是单条权限声明的展示形态。
type PermissionSummary struct {
	Permission string   `json:"permission"`
	Scope      string   `json:"scope"`
	Actions    []string `json:"actions"`
}

// ExtensionVersionSummary 是详情页版本列表行。
type ExtensionVersionSummary struct {
	extension.Version
	ManifestSummary VersionManifestSummary `json:"manifestSummary"`
	Permissions     []PermissionSummary    `json:"permissions"`
}

// ExtensionDetail 是扩展详情：扩展信息 + 全部版本摘要与权限清单。
// H7.1：附带当前租户安装状态。
type ExtensionDetail struct {
	extension.Extension
	Versions         []ExtensionVersionSummary `json:"versions"`
	Installed        bool                      `json:"installed"`
	InstalledVersion string                    `json:"installedVersion"`
	InstalledStatus  string                    `json:"installedStatus"`
}

// Get 返回租户内扩展详情（含版本列表、各版本 manifest 摘要与权限清单）。
func (s *ExtensionService) Get(tenantID string, id uint64) (*ExtensionDetail, error) {
	var ext extension.Extension
	if err := s.db.Where("tenant_id=? AND id=?", tenantID, id).First(&ext).Error; err != nil {
		return nil, err
	}
	var versions []extension.Version
	if err := s.db.Where("extension_id=?", ext.ID).Order("created_at DESC").Find(&versions).Error; err != nil {
		return nil, err
	}
	detail := &ExtensionDetail{Extension: ext, Versions: make([]ExtensionVersionSummary, 0, len(versions))}
	// H7.1：附带当前租户安装状态
	var inst extension.Install
	if err := s.db.Where("tenant_id=? AND extension_id=?", tenantID, ext.ID).First(&inst).Error; err == nil {
		detail.Installed = true
		detail.InstalledVersion = inst.Version
		detail.InstalledStatus = inst.Status
	}
	for _, v := range versions {
		detail.Versions = append(detail.Versions, ExtensionVersionSummary{
			Version:         v,
			ManifestSummary: summarizeManifest(v.Manifest),
			Permissions:     summarizePermissions(v.Manifest),
		})
	}
	return detail, nil
}

// GetVersion 返回单个版本的完整 manifest。
func (s *ExtensionService) GetVersion(tenantID string, id uint64, version string) (*extension.Version, error) {
	var ext extension.Extension
	if err := s.db.Where("tenant_id=? AND id=?", tenantID, id).First(&ext).Error; err != nil {
		return nil, err
	}
	var ver extension.Version
	if err := s.db.Where("extension_id=? AND version=?", ext.ID, version).First(&ver).Error; err != nil {
		return nil, err
	}
	return &ver, nil
}

// GetEnabled 提供与 H6 CapabilityRegistryService.GetEnabled 等价的语义：
// 返回租户内指定扩展（按 DNS 名）指定版本的启用状态记录；未启用或不存在报错。
// H7.1 起"启用"以 extension_installs（status=enabled 且版本匹配）为准：
// 停用只影响新事件生效，历史 run_states 数据依旧可读。
// 查询走 extensions + extension_versions + extension_installs 新表，
// H6 的 capability_packages 读取不受影响。
func (s *ExtensionService) GetEnabled(tx *gorm.DB, tenantID, name, version string) (*extension.Extension, *extension.Version, error) {
	var ext extension.Extension
	err := tx.Where("tenant_id=? AND name=? AND status=?", tenantID, name, extension.StatusActive).First(&ext).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, fmt.Errorf("扩展 %s@%s 未注册或未启用", name, version)
	}
	if err != nil {
		return nil, nil, err
	}
	var inst extension.Install
	err = tx.Where("tenant_id=? AND extension_id=? AND status=?", tenantID, ext.ID, extension.InstallStatusEnabled).
		First(&inst).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, fmt.Errorf("扩展 %s@%s 未安装或未启用", name, version)
	}
	if err != nil {
		return nil, nil, err
	}
	if version != "" && inst.Version != version {
		return nil, nil, fmt.Errorf("扩展 %s@%s 未启用（当前安装版本为 %s）", name, version, inst.Version)
	}
	var ver extension.Version
	if err := tx.Where("extension_id=? AND version=?", ext.ID, inst.Version).First(&ver).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, fmt.Errorf("扩展 %s@%s 未注册或未启用", name, version)
		}
		return nil, nil, err
	}
	return &ext, &ver, nil
}

func summarizeManifest(raw string) VersionManifestSummary {
	manifest, errs := extensionmanifest.ValidateExtensionManifest([]byte(raw))
	if len(errs) > 0 || manifest == nil {
		// P1：失败分支也要交出空切片而不是零值 —— VersionManifestSummary 的
		// Slots 没有 omitempty，零值会序列化成 `null`，前端 `.length` / `.map()`
		// 直接抛错被根 ErrorBoundary 接住 → 整页白屏。
		return VersionManifestSummary{Slots: []string{}}
	}
	summary := VersionManifestSummary{
		DisplayName:          manifest.DisplayName,
		Description:          manifest.Description,
		StateSchemaCount:     len(manifest.StateSchemas),
		EventCount:           len(manifest.Events),
		ToolCount:            len(manifest.Tools),
		RelationCount:        len(manifest.Relations),
		PromptInjectionCount: len(manifest.PromptInjections),
		// P1：列表字段永不序列化成 null。前端按 `x.length` / `x.map()` 使用，
		// null 会变成 `undefined.length` 抛错被根 ErrorBoundary 接住 → 整页白屏
		// （docs/h7-acceptance.md 记录的 belief disputes 事故是同一形态）。
		Slots: []string{},
	}
	if manifest.UI != nil {
		summary.Slots = manifest.UI.SlotNames()
	}
	return summary
}

func summarizePermissions(raw string) []PermissionSummary {
	manifest, errs := extensionmanifest.ValidateExtensionManifest([]byte(raw))
	if len(errs) > 0 || manifest == nil {
		// 同上：返回空切片而不是 nil。manifest 校验失败的扩展在列表页仍要能渲染，
		// 只是权限清单显示"未声明"。
		return []PermissionSummary{}
	}
	out := make([]PermissionSummary, 0, len(manifest.Permissions))
	for _, p := range manifest.Permissions {
		actions := p.Actions
		if actions == nil {
			actions = []string{}
		}
		out = append(out, PermissionSummary{Permission: p.Permission, Scope: p.Scope, Actions: actions})
	}
	return out
}
