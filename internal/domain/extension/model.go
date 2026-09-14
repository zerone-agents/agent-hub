// Package extension 定义 H7 扩展注册中心的领域模型。
//
// Extension 表示一个租户域内的扩展（名称全局 DNS 式唯一，租户内唯一），
// ExtensionVersion 表示该扩展的一个不可变语义化版本，完整 manifest 以
// 规范 JSON 形式落库并按内容寻址（content_hash）。平台级扩展的 tenant_id
// 固定为 "default"。模型完全通用，不包含任何垂直业务语义。
package extension

import "time"

// Source 取值：扩展的来源渠道。
const (
	SourceSeed     = "seed"
	SourceRegistry = "registry"
	SourceUpload   = "upload"
)

// Status 取值：扩展生命周期状态（H7.0 注册中心只产生 active，
// 其余状态由 H7.1 生命周期阶段接管）。
const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
	StatusDraft    = "draft"
)

// Extension 是扩展注册记录，一个扩展持有若干不可变版本。
type Extension struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID    string    `gorm:"type:varchar(64);not null;uniqueIndex:uk_extensions_tenant_name,priority:1;index" json:"-"`
	Name        string    `gorm:"type:varchar(253);not null;uniqueIndex:uk_extensions_tenant_name,priority:2" json:"name"`
	DisplayName string    `gorm:"type:varchar(160);not null;default:''" json:"displayName"`
	Description string    `gorm:"type:text;not null" json:"description"`
	Icon        string    `gorm:"type:varchar(512);not null;default:''" json:"icon"`
	Source      string    `gorm:"type:varchar(32);not null;default:'upload';index" json:"source"`
	Status      string    `gorm:"type:varchar(24);not null;default:'active';index" json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (Extension) TableName() string { return "extensions" }

// Version 是扩展的一个不可变版本；同一 (extension_id, version) 的内容
// 必须一致，重复提交相同内容按幂等返回既有版本。
type Version struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ExtensionID uint64    `gorm:"not null;uniqueIndex:uk_extension_versions,priority:1;index" json:"extensionId"`
	Version     string    `gorm:"type:varchar(64);not null;uniqueIndex:uk_extension_versions,priority:2" json:"version"`
	Manifest    string    `gorm:"type:longtext;not null" json:"manifest"`
	ContentHash string    `gorm:"type:char(64);not null;index" json:"contentHash"`
	Changelog   string    `gorm:"type:text;not null" json:"changelog"`
	CreatedBy   string    `gorm:"type:varchar(160);not null;default:''" json:"createdBy"`
	CreatedAt   time.Time `json:"createdAt"`
}

func (Version) TableName() string { return "extension_versions" }
