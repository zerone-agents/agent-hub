// Package template 定义 H7.3 模板库的领域模型。
//
// 模板是只包含数据的声明式配置（agents / groups / relations / workflows /
// stateSchemas / sampleData 等），MUST NOT 执行代码、注册工具或授予权限。
// TemplateDefinition 是模板头（租户内 name 唯一），TemplateVersion 是
// 不可变 spec 快照（tenant 内 template_id+version 唯一，内容寻址 content_hash），
// TemplateInstall 是安装幂等记录（idempotency_key 唯一，同 key 同 mapping 重试
// 返回首次结果）。模型完全通用，不包含任何垂直业务语义。
package template

import "time"

// 模板分类取值（可扩展，注册时校验在常用集合内）。
const (
	CategoryTeam    = "team"
	CategoryGame    = "game"
	CategoryGeneral = "general"
)

// Source 取值：模板来源渠道。
const (
	SourceSeed = "seed"
	SourceUser = "user"
)

// TemplateDefinition 是模板注册记录，一个模板持有若干不可变版本。
type TemplateDefinition struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID    string    `gorm:"type:varchar(64);not null;uniqueIndex:uk_template_defs_tenant_name,priority:1;index" json:"-"`
	Name        string    `gorm:"type:varchar(253);not null;uniqueIndex:uk_template_defs_tenant_name,priority:2" json:"name"`
	DisplayName string    `gorm:"type:varchar(160);not null;default:''" json:"displayName"`
	Description string    `gorm:"type:text;not null" json:"description"`
	Category    string    `gorm:"type:varchar(32);not null;default:'general';index" json:"category"`
	Icon        string    `gorm:"type:varchar(512);not null;default:''" json:"icon"`
	Source      string    `gorm:"type:varchar(16);not null;default:'user';index" json:"source"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (TemplateDefinition) TableName() string { return "template_definitions" }

// TemplateVersion 是模板的一个不可变 spec 版本；同一 (template_id, version)
// 的内容必须一致（内容寻址），重复提交相同内容按幂等返回既有版本。
type TemplateVersion struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	TemplateID  uint64    `gorm:"not null;uniqueIndex:uk_template_versions,priority:1;index" json:"templateId"`
	Version     string    `gorm:"type:varchar(64);not null;uniqueIndex:uk_template_versions,priority:2" json:"version"`
	Spec        string    `gorm:"type:longtext;not null" json:"spec"`
	ContentHash string    `gorm:"type:char(64);not null;index" json:"contentHash"`
	CreatedBy   string    `gorm:"type:varchar(160);not null;default:''" json:"createdBy"`
	CreatedAt   time.Time `json:"createdAt"`
}

func (TemplateVersion) TableName() string { return "template_versions" }

// TemplateInstall 是一次模板安装的幂等记录：同 (tenant, idempotency_key)
// 至多一行；同 key 同 mapping_hash 重试直接返回首次 result，同 key 不同
// mapping 报 409。并发安装靠唯一索引保证只有一个写入成功。
type TemplateInstall struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID       string    `gorm:"type:varchar(64);not null;uniqueIndex:uk_template_installs_key,priority:1;index" json:"-"`
	TemplateID     uint64    `gorm:"not null;index" json:"templateId"`
	TemplateName   string    `gorm:"type:varchar(253);not null" json:"templateName"`
	Version        string    `gorm:"type:varchar(64);not null" json:"version"`
	MappingHash    string    `gorm:"type:char(64);not null" json:"mappingHash"`
	IdempotencyKey string    `gorm:"column:idempotency_key;type:varchar(191);not null;uniqueIndex:uk_template_installs_key,priority:2" json:"idempotencyKey"`
	Result         string    `gorm:"type:longtext;not null" json:"result"`
	CreatedAt      time.Time `json:"createdAt"`
}

func (TemplateInstall) TableName() string { return "template_installs" }
