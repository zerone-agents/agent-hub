// Package extensionslot 定义 H7.2 UI 插槽的领域模型。
//
// SlotItem 是从扩展 manifest ui.slots 声明聚合出的渲染单元；
// Override 是租户级的临时隐藏记录（extension_slot_overrides 表，
// tenant_id+extension_name+slot+component 唯一）。扩展不执行任意代码，
// 插槽内容由声明式渲染协议消费（见 internal/extensionmanifest）。
package extensionslot

import "time"

// Item 是聚合后的单个插槽组件声明（含来源扩展信息）。
type Item struct {
	Slot          string         `json:"slot"`
	Component     string         `json:"component"` // stat-card/link-list/key-value/markdown
	Title         string         `json:"title"`
	Order         int            `json:"order"`
	Visible       bool           `json:"visible"` // 已应用 override 后的最终可见性
	Data          map[string]any `json:"data,omitempty"`
	DataSource    *DataSource    `json:"dataSource,omitempty"`
	ExtensionName string         `json:"extensionName"`
	ExtensionID   uint64         `json:"extensionId"`
	Version       string         `json:"version"`
}

// DataSource 指向扩展自己声明的 GET 授权 API 端点。
type DataSource struct {
	Path string `json:"path"`
}

// Override 是租户级插槽可见性覆盖（临时隐藏/恢复某个组件）。
type Override struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID      string    `gorm:"type:varchar(64);not null;uniqueIndex:uk_extension_slot_overrides,priority:1" json:"-"`
	ExtensionName string    `gorm:"type:varchar(253);not null;uniqueIndex:uk_extension_slot_overrides,priority:2" json:"extensionName"`
	Slot          string    `gorm:"type:varchar(64);not null;uniqueIndex:uk_extension_slot_overrides,priority:3" json:"slot"`
	Component     string    `gorm:"type:varchar(64);not null;uniqueIndex:uk_extension_slot_overrides,priority:4" json:"component"`
	Visible       bool      `gorm:"not null" json:"visible"`
	UpdatedBy     string    `gorm:"type:varchar(160);not null;default:''" json:"updatedBy"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

func (Override) TableName() string { return "extension_slot_overrides" }
