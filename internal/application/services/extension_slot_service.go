// ExtensionSlotService 是 H7.2 UI 插槽注册表：
// 从已启用扩展的当前安装版本 manifest 聚合声明式 ui.slots 挂载，
// 应用租户级 extension_slot_overrides 可见性覆盖后按 (order, 扩展名, title)
// 排序返回；同时提供授权 API 路由解析，供代理 handler 按声明转发。
package services

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"control-panel/internal/domain/extension"
	"control-panel/internal/domain/extensionslot"
	"control-panel/internal/extensionmanifest"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ExtensionSlotService struct{ db *gorm.DB }

func NewExtensionSlotService(db *gorm.DB) *ExtensionSlotService {
	return &ExtensionSlotService{db: db}
}

// List 返回租户内某插槽的聚合组件列表（已启用扩展 + override 已应用）。
// slot 为空串时返回全部插槽。默认 visible=false 的声明不出现在结果中；
// 有 override 时以 override 为准。
func (s *ExtensionSlotService) List(tenantID, slot string) ([]extensionslot.Item, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		tenantID = "default"
	}
	slot = strings.TrimSpace(slot)

	var installs []extension.Install
	if err := s.db.Where("tenant_id=? AND status=?", tenantID, extension.InstallStatusEnabled).Find(&installs).Error; err != nil {
		return nil, err
	}

	var overrides []extensionslot.Override
	if err := s.db.Where("tenant_id=?", tenantID).Find(&overrides).Error; err != nil {
		return nil, err
	}
	overrideKey := func(extName, sl, comp string) *bool {
		for i := range overrides {
			o := &overrides[i]
			if o.ExtensionName == extName && o.Slot == sl && o.Component == comp {
				return &o.Visible
			}
		}
		return nil
	}

	items := make([]extensionslot.Item, 0)
	for _, inst := range installs {
		var ext extension.Extension
		if err := s.db.Where("id=? AND tenant_id=? AND status=?", inst.ExtensionID, tenantID, extension.StatusActive).First(&ext).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue // 扩展已不存在或非 active
			}
			return nil, err
		}
		var ver extension.Version
		if err := s.db.Where("extension_id=? AND version=?", ext.ID, inst.Version).First(&ver).Error; err != nil {
			continue // 安装版本缺失，跳过
		}
		manifest, errs := extensionmanifest.ValidateExtensionManifest([]byte(ver.Manifest))
		if len(errs) > 0 || manifest == nil || manifest.UI == nil {
			continue
		}
		for _, decl := range manifest.UI.Slots {
			if decl.Component == "" {
				continue // 纯字符串形态：不渲染
			}
			if slot != "" && decl.Slot != slot {
				continue
			}
			visible := decl.Visible == nil || *decl.Visible
			if ov := overrideKey(ext.Name, decl.Slot, decl.Component); ov != nil {
				visible = *ov
			}
			if !visible {
				continue
			}
			item := extensionslot.Item{
				Slot:          decl.Slot,
				Component:     decl.Component,
				Title:         decl.Title,
				Order:         decl.Order,
				Visible:       visible,
				Data:          decl.Data,
				ExtensionName: ext.Name,
				ExtensionID:   ext.ID,
				Version:       ver.Version,
			}
			if decl.DataSource != nil {
				item.DataSource = &extensionslot.DataSource{Path: decl.DataSource.Path}
			}
			items = append(items, item)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Order != items[j].Order {
			return items[i].Order < items[j].Order
		}
		if items[i].ExtensionName != items[j].ExtensionName {
			return items[i].ExtensionName < items[j].ExtensionName
		}
		return items[i].Title < items[j].Title
	})
	return items, nil
}

// SetVisible 写入租户级可见性覆盖（upsert，唯一键冲突时更新）。
func (s *ExtensionSlotService) SetVisible(tenantID, extName, slot, component string, visible bool, actor string) (*extensionslot.Override, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		tenantID = "default"
	}
	extName = strings.TrimSpace(extName)
	slot = strings.TrimSpace(slot)
	component = strings.TrimSpace(component)
	if extName == "" || slot == "" || component == "" {
		return nil, fmt.Errorf("extensionName、slot、component 均不能为空")
	}
	if !containsSlotName(extensionmanifest.UISlots, slot) {
		return nil, fmt.Errorf("slot %q 不是合法插槽", slot)
	}
	if !containsSlotName(extensionmanifest.UIComponentTypes, component) {
		return nil, fmt.Errorf("component %q 不是合法组件类型", component)
	}
	ov := extensionslot.Override{
		TenantID:      tenantID,
		ExtensionName: extName,
		Slot:          slot,
		Component:     component,
		Visible:       visible,
		UpdatedBy:     strings.TrimSpace(actor),
	}
	err := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "extension_name"}, {Name: "slot"}, {Name: "component"}},
		DoUpdates: clause.AssignmentColumns([]string{"visible", "updated_by", "updated_at"}),
	}).Create(&ov).Error
	if err != nil {
		return nil, err
	}
	return &ov, nil
}

func containsSlotName(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

// ExtensionNameByID 按租户内扩展 ID 查扩展名（供 override 端点把 :id 映射为 name）。
func (s *ExtensionSlotService) ExtensionNameByID(tenantID string, id uint64) (string, error) {
	var ext extension.Extension
	if err := s.db.Where("tenant_id=? AND id=?", tenantID, id).First(&ext).Error; err != nil {
		return "", err
	}
	return ext.Name, nil
}

// ResolvedRoute 是解析出的代理目标。
type ResolvedRoute struct {
	Upstream string
}

// ResolveAPIRoute 按租户+扩展名+方法+路径解析声明的授权 API 路由。
// 未声明一律报错（由 handler 映射 403，绝不转发到任意上游）。
func (s *ExtensionSlotService) ResolveAPIRoute(tenantID, name, method, path string) (*ResolvedRoute, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		tenantID = "default"
	}
	name = strings.TrimSpace(name)
	path = strings.TrimSpace(path)
	// 防路径穿越：声明路径是精确匹配的，这里再兜一层
	if strings.Contains(path, "..") || !strings.HasPrefix(path, "/api/v1/extensions/"+name+"/") {
		return nil, fmt.Errorf("路径不在扩展 %s 的授权范围内", name)
	}
	var ext extension.Extension
	if err := s.db.Where("tenant_id=? AND name=? AND status=?", tenantID, name, extension.StatusActive).First(&ext).Error; err != nil {
		return nil, fmt.Errorf("扩展 %s 不存在或未启用", name)
	}
	var inst extension.Install
	if err := s.db.Where("tenant_id=? AND extension_id=? AND status=?", tenantID, ext.ID, extension.InstallStatusEnabled).First(&inst).Error; err != nil {
		return nil, fmt.Errorf("扩展 %s 未安装或未启用", name)
	}
	var ver extension.Version
	if err := s.db.Where("extension_id=? AND version=?", ext.ID, inst.Version).First(&ver).Error; err != nil {
		return nil, fmt.Errorf("扩展 %s 的安装版本 %s 不存在", name, inst.Version)
	}
	manifest, errs := extensionmanifest.ValidateExtensionManifest([]byte(ver.Manifest))
	if len(errs) > 0 || manifest == nil {
		return nil, fmt.Errorf("扩展 %s 的 manifest 无法解析", name)
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	for _, r := range manifest.APIRoutes {
		if r.Path == path && strings.ToUpper(strings.TrimSpace(r.Method)) == method {
			return &ResolvedRoute{Upstream: strings.TrimSpace(r.Upstream)}, nil
		}
	}
	return nil, fmt.Errorf("扩展 %s 未声明 %s %s 授权端点", name, method, path)
}
