// H7.3 模板库应用服务：模板注册/查询、安装预览（Resolve）、一键安装
// （Install，单事务 + 幂等 + 冲突策略 + 扩展依赖检查 + 部分 section）、
// 以及从当前配置反向导出（Export）。模板安装只创建配置/实例，不执行代码、
// 不注册工具、不授予权限（见 platform-extension-protocol §3.1）。
package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"control-panel/internal/domain/extension"
	"control-panel/internal/domain/template"

	"gorm.io/gorm"
)

// TemplateError 是模板业务错误，携带建议 HTTP 状态码与中文说明；
// Plan 可选地携带冲突预览（409 场景）。
type TemplateError struct {
	Code    int
	Message string
	Plan    *InstallPlan
}

func (e *TemplateError) Error() string { return e.Message }

func templateErrorf(code int, format string, args ...any) *TemplateError {
	return &TemplateError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// IsTemplateError 判断 err 并返回状态码。
func IsTemplateError(err error) (int, bool) {
	var te *TemplateError
	if errors.As(err, &te) {
		return te.Code, true
	}
	return 0, false
}

// TemplateService 是模板库服务。run 可为 nil：为 nil 时 sampleData
// 只在提供 targetRunID 时写库，否则统一标注 skipped。
type TemplateService struct {
	db        *gorm.DB
	run       *RunService
	lifecycle *ExtensionLifecycleService
}

func NewTemplateService(db *gorm.DB) *TemplateService {
	return &TemplateService{db: db, lifecycle: NewExtensionLifecycleService(db)}
}

// SetRunService 注入 RunService 以支持 sampleData 写入目标运行。
func (s *TemplateService) SetRunService(run *RunService) { s.run = run }

// ---------- 注册与查询 ----------

type RegisterTemplateInput struct {
	Name        string // DNS 式模板名（租户内唯一）
	DisplayName string
	Description string
	Category    string // team/game/general
	Icon        string
	Version     string // semver
	Spec        json.RawMessage
	Source      string // seed/user，默认 user
	CreatedBy   string
}

type RegisterTemplateResult struct {
	Template       *template.TemplateDefinition
	Version        *template.TemplateVersion
	AlreadyExisted bool
}

// Register 注册模板 + 版本：spec 严格校验（中文错误一次收集）；
// 同 (tenant, name) 复用定义行；同 (template, version, content_hash) 幂等
// 返回既有版本；同版本不同内容报 409 式冲突错误。
func (s *TemplateService) Register(tenantID string, in RegisterTemplateInput) (*RegisterTemplateResult, error) {
	tenantID = normalizedTenant(tenantID)
	name := strings.TrimSpace(in.Name)
	if !templateDNSNameName(name) {
		return nil, templateErrorf(http.StatusBadRequest, "模板 name %q 不符合 DNS 式命名（如 io.zerone.team）", name)
	}
	category := strings.TrimSpace(in.Category)
	if category == "" {
		category = template.CategoryGeneral
	}
	switch category {
	case template.CategoryTeam, template.CategoryGame, template.CategoryGeneral:
	default:
		return nil, templateErrorf(http.StatusBadRequest, "category 必须是 team/game/general 之一，当前为 %q", category)
	}
	if strings.TrimSpace(in.Version) == "" || !extension.IsValidVersion(in.Version) {
		return nil, templateErrorf(http.StatusBadRequest, "version %q 不是合法语义化版本", in.Version)
	}
	var spec template.Spec
	if err := json.Unmarshal(in.Spec, &spec); err != nil {
		return nil, templateErrorf(http.StatusBadRequest, "spec 必须是合法 JSON：%v", err)
	}
	if errs := template.ValidateTemplateSpec(&spec); len(errs) > 0 {
		return nil, templateErrorf(http.StatusBadRequest, "模板 spec 校验失败：%s", strings.Join(errs, "；"))
	}
	hash := specContentHash(in.Spec)
	source := strings.TrimSpace(in.Source)
	if source == "" {
		source = template.SourceUser
	}
	switch source {
	case template.SourceSeed, template.SourceUser:
	default:
		return nil, templateErrorf(http.StatusBadRequest, "source 必须是 seed/user 之一，当前为 %q", source)
	}

	result := &RegisterTemplateResult{}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var def template.TemplateDefinition
		err := tx.Where("tenant_id=? AND name=?", tenantID, name).First(&def).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			def = template.TemplateDefinition{
				TenantID: tenantID, Name: name,
				DisplayName: strings.TrimSpace(in.DisplayName),
				Description: strings.TrimSpace(in.Description),
				Category:    category, Icon: strings.TrimSpace(in.Icon),
				Source: source,
			}
			if err := tx.Create(&def).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			// 展示字段只以非空输入覆盖（幂等重注册不应清空既有元信息）；
			// category 为空时保持既有分类。
			updates := map[string]any{}
			if v := strings.TrimSpace(in.DisplayName); v != "" {
				updates["display_name"] = v
			}
			if v := strings.TrimSpace(in.Description); v != "" {
				updates["description"] = v
			}
			if strings.TrimSpace(in.Category) != "" {
				updates["category"] = category
			}
			if v := strings.TrimSpace(in.Icon); v != "" {
				updates["icon"] = v
			}
			if len(updates) > 0 {
				if err := tx.Model(&template.TemplateDefinition{}).Where("id=?", def.ID).Updates(updates).Error; err != nil {
					return err
				}
			}
		}

		var ver template.TemplateVersion
		err = tx.Where("template_id=? AND version=?", def.ID, in.Version).First(&ver).Error
		switch {
		case err == nil:
			if ver.ContentHash != hash {
				return templateErrorf(http.StatusConflict,
					"模板 %s 的版本 %s 已存在且内容不一致（内容寻址冲突）", name, in.Version)
			}
			result.AlreadyExisted = true
			result.Version = &ver
		case errors.Is(err, gorm.ErrRecordNotFound):
			ver = template.TemplateVersion{
				TemplateID: def.ID, Version: in.Version,
				Spec: string(in.Spec), ContentHash: hash,
				CreatedBy: strings.TrimSpace(in.CreatedBy),
			}
			if err := tx.Create(&ver).Error; err != nil {
				return err
			}
			result.Version = &ver
		default:
			return err
		}
		result.Template = &def
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func templateDNSNameName(name string) bool {
	if name == "" || len(name) > 253 {
		return false
	}
	// 与 extensionmanifest 的 DNS 式命名保持一致。
	labels := strings.Split(name, ".")
	for _, l := range labels {
		if l == "" || len(l) > 63 || l[0] < 'a' || l[0] > 'z' {
			return false
		}
		for _, r := range l[1:] {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
				continue
			}
			return false
		}
	}
	return len(labels) >= 2
}

// specContentHash 计算 spec 规范 JSON 的内容哈希（json.Marshal 对 map
// key 排序，保证同一语义内容哈希稳定）。
func specContentHash(raw json.RawMessage) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		sum := sha256.Sum256(raw)
		return hex.EncodeToString(sum[:])
	}
	canonical, _ := json.Marshal(v)
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

// mappingHash 计算 mapping 的规范哈希。
func mappingHash(m TemplateMapping) string {
	canonical, _ := json.Marshal(m)
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

type TemplateListFilter struct {
	Category string
	Page     int
	PageSize int
}

type TemplateListItem struct {
	template.TemplateDefinition
	VersionCount  int64  `json:"versionCount"`
	LatestVersion string `json:"latestVersion"`
}

type TemplateListPage struct {
	Items    []TemplateListItem `json:"items"`
	Total    int64              `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"pageSize"`
}

// findDefinition 查找模板定义：先本租户，未命中回退共享租户 "default"
// 的模板（共享内置语义：各租户可读可装，安装产物落在请求租户内）。
func (s *TemplateService) findDefinition(tenantID string, id uint64) (*template.TemplateDefinition, error) {
	var def template.TemplateDefinition
	err := s.db.Where("tenant_id=? AND id=?", normalizedTenant(tenantID), id).First(&def).Error
	if errors.Is(err, gorm.ErrRecordNotFound) && normalizedTenant(tenantID) != "default" {
		err = s.db.Where("tenant_id=? AND id=?", "default", id).First(&def).Error
	}
	if err != nil {
		return nil, err
	}
	return &def, nil
}

// List 按租户隔离列出模板（含共享租户种子），支持 category 过滤与分页。
func (s *TemplateService) List(tenantID string, filter TemplateListFilter) (*TemplateListPage, error) {
	tenantID = normalizedTenant(tenantID)
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	query := func(t string) *gorm.DB {
		base := s.db.Model(&template.TemplateDefinition{}).Where("tenant_id=?", t)
		if filter.Category != "" {
			base = base.Where("category=?", filter.Category)
		}
		return base
	}
	var defs []template.TemplateDefinition
	var total int64
	if err := query(tenantID).Order("name").Find(&defs).Error; err != nil {
		return nil, err
	}
	if err := query(tenantID).Count(&total).Error; err != nil {
		return nil, err
	}
	if tenantID != "default" {
		var shared []template.TemplateDefinition
		if err := query("default").Order("name").Find(&shared).Error; err != nil {
			return nil, err
		}
		var sharedTotal int64
		if err := query("default").Count(&sharedTotal).Error; err != nil {
			return nil, err
		}
		defs = append(defs, shared...)
		total += sharedTotal
	}
	// 共享与租户同名时租户版本优先（按 name 去重）
	byName := map[string]bool{}
	unique := make([]template.TemplateDefinition, 0, len(defs))
	for _, d := range defs {
		if byName[d.Name] {
			continue
		}
		byName[d.Name] = true
		unique = append(unique, d)
	}
	// 分页在合并后应用
	start := (filter.Page - 1) * filter.PageSize
	if start > len(unique) {
		start = len(unique)
	}
	end := start + filter.PageSize
	if end > len(unique) {
		end = len(unique)
	}
	paged := unique[start:end]
	items := make([]TemplateListItem, 0, len(paged))
	for _, def := range paged {
		var count int64
		var latest template.TemplateVersion
		if err := s.db.Model(&template.TemplateVersion{}).Where("template_id=?", def.ID).Count(&count).Error; err != nil {
			return nil, err
		}
		_ = s.db.Where("template_id=?", def.ID).Order("created_at DESC").First(&latest).Error
		items = append(items, TemplateListItem{TemplateDefinition: def, VersionCount: count, LatestVersion: latest.Version})
	}
	return &TemplateListPage{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

// TemplateVersionItem 是详情页版本行（spec 计数摘要）。
type TemplateVersionItem struct {
	template.TemplateVersion
	Summary SpecSummary `json:"summary"`
}

// SpecSummary 是 spec 的内容摘要（详情页/卡片展示将创建什么）。
type SpecSummary struct {
	Agents               int `json:"agents"`
	PersonalityTemplates int `json:"personalityTemplates"`
	Groups               int `json:"groups"`
	Relations            int `json:"relations"`
	Workflows            int `json:"workflows"`
	StateSchemas         int `json:"stateSchemas"`
	ExtensionDeps        int `json:"extensionDeps"`
	SampleData           int `json:"sampleData"`
}

func summarizeSpec(spec *template.Spec) SpecSummary {
	if spec == nil {
		return SpecSummary{}
	}
	return SpecSummary{
		Agents: len(spec.Agents), PersonalityTemplates: len(spec.PersonalityTemplates),
		Groups: len(spec.Groups), Relations: len(spec.Relations),
		Workflows: len(spec.Workflows), StateSchemas: len(spec.StateSchemas),
		ExtensionDeps: len(spec.ExtensionDeps), SampleData: len(spec.SampleData),
	}
}

// TemplateDetail 是模板详情：定义 + 版本列表（含 spec 摘要）。
type TemplateDetail struct {
	template.TemplateDefinition
	Versions []TemplateVersionItem `json:"versions"`
}

func (s *TemplateService) Get(tenantID string, id uint64) (*TemplateDetail, error) {
	def, err := s.findDefinition(tenantID, id)
	if err != nil {
		return nil, err
	}
	var versions []template.TemplateVersion
	if err := s.db.Where("template_id=?", def.ID).Order("created_at DESC").Find(&versions).Error; err != nil {
		return nil, err
	}
	detail := &TemplateDetail{TemplateDefinition: *def, Versions: make([]TemplateVersionItem, 0, len(versions))}
	for _, v := range versions {
		var spec template.Spec
		_ = json.Unmarshal([]byte(v.Spec), &spec)
		detail.Versions = append(detail.Versions, TemplateVersionItem{TemplateVersion: v, Summary: summarizeSpec(&spec)})
	}
	return detail, nil
}

// GetVersion 返回单个模板版本的完整 spec。
func (s *TemplateService) GetVersion(tenantID string, id uint64, version string) (*template.TemplateVersion, error) {
	def, err := s.findDefinition(tenantID, id)
	if err != nil {
		return nil, err
	}
	var ver template.TemplateVersion
	if err := s.db.Where("template_id=? AND version=?", def.ID, strings.TrimSpace(version)).First(&ver).Error; err != nil {
		return nil, err
	}
	return &ver, nil
}

func (s *TemplateService) loadVersion(tenantID string, templateID uint64, version string) (*template.TemplateDefinition, *template.TemplateVersion, *template.Spec, error) {
	def, err := s.findDefinition(tenantID, templateID)
	if err != nil {
		return nil, nil, nil, err
	}
	q := s.db.Where("template_id=?", def.ID)
	version = strings.TrimSpace(version)
	if version == "" {
		q = q.Order("created_at DESC")
	} else {
		q = q.Where("version=?", version)
	}
	var ver template.TemplateVersion
	if err := q.First(&ver).Error; err != nil {
		return nil, nil, nil, err
	}
	var spec template.Spec
	if err := json.Unmarshal([]byte(ver.Spec), &spec); err != nil {
		return nil, nil, nil, templateErrorf(http.StatusInternalServerError, "模板 spec 解析失败：%v", err)
	}
	if errs := template.ValidateTemplateSpec(&spec); len(errs) > 0 {
		return nil, nil, nil, templateErrorf(http.StatusBadRequest, "模板 spec 校验失败：%s", strings.Join(errs, "；"))
	}
	return def, &ver, &spec, nil
}
