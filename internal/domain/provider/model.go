package provider

import (
	"errors"
	"time"
)

var ErrProviderNotFound = errors.New("provider not found")

// ── Enum types ──────────────────────────────────────────────────

type Protocol string

const (
	ProtocolAnthropic Protocol = "anthropic"
	ProtocolOpenAI    Protocol = "openai"
	ProtocolMinerU    Protocol = "mineru"
	ProtocolPaddleOCR Protocol = "paddleocr"
)

type AuthStyle string

const (
	AuthStyleAPIKey    AuthStyle = "api_key"
	AuthStyleAuthToken AuthStyle = "auth_token"
	AuthStyleNoAuth    AuthStyle = "no_auth"
)

type ProviderType string

const (
	TypeLLM       ProviderType = "llm"
	TypeOCR       ProviderType = "ocr"
	TypeEmbedding ProviderType = "embedding"
	// TypeVLM marks multimodal vision-language models. They sync to
	// MultiRAG as mdl_type "image2text" and are also eligible for Agent
	// binding (alongside TypeLLM).
	TypeVLM ProviderType = "vlm"
)

// ── Nested types (stored as JSON in DB) ─────────────────────────

type CatalogModel struct {
	SelectionID   string   `json:"selectionId,omitempty"`
	ModelID       string   `json:"modelId"`
	DisplayName   string   `json:"displayName"`
	ContextWindow int      `json:"contextWindow,omitempty"`
	ModelType     string   `json:"modelType,omitempty"`
	Status        string   `json:"status,omitempty"`
	Efforts       []string `json:"efforts,omitempty"`
	AigcCode      string   `json:"aigcCode,omitempty"`
}

type PresetFieldOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type PresetField struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	LabelEn  string `json:"labelEn"`
	Type     string `json:"type"`
	Required bool   `json:"required,omitempty"`
	Secret   bool   `json:"secret,omitempty"`
}

// AttrValue is the weakly-typed value stored in the EAV attribute table.
// Value is always a string; Type tells the reader how to interpret it.
type AttrValue struct {
	Type  string `json:"type"` // "string" | "bool" | "int"
	Value string `json:"value"`
}

// ── DB entities ──────────────────────────────────────────────────

// ProviderSummary is the new primary table — the "necessary descriptive
// info" for a vendor preset. Extensible per-provider config lives in the
// provider_attributes EAV table instead of as fixed columns.
type ProviderSummary struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID      string    `gorm:"type:varchar(64);not null;default:'';uniqueIndex:uk_tenant_key,priority:1;index" json:"-"`
	Key           string    `gorm:"type:varchar(64);uniqueIndex:uk_tenant_key,priority:2;not null" json:"key"`
	Name          string    `gorm:"type:varchar(128);not null" json:"name"`
	Description   string    `gorm:"type:text" json:"description"`
	DescriptionEn string    `gorm:"column:description_en;type:text" json:"descriptionEn"`
	Protocol      string    `gorm:"type:varchar(16);not null" json:"protocol"`
	AuthStyle     string    `gorm:"type:varchar(16);not null" json:"authStyle"`
	BaseURL       string    `gorm:"column:base_url;type:varchar(512)" json:"baseUrl"`
	Fields        string    `gorm:"column:fields;type:text" json:"-"`
	IconKey       string    `gorm:"column:icon_key;type:varchar(32)" json:"iconKey"`
	Builtin       bool      `gorm:"default:false" json:"builtin"`
	LockedAPIKey  string    `gorm:"column:locked_api_key;type:text" json:"-"` // encrypted
	CreatedAt     time.Time `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt     time.Time `gorm:"column:updated_at;index" json:"updatedAt"`
}

func (ProviderSummary) TableName() string {
	return "provider_summaries"
}

// ProviderAttribute is one row of the EAV table holding a single
// provider-specific config attribute. (provider_id, attr_key) is unique.
type ProviderAttribute struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	ProviderID uint64 `gorm:"column:provider_id;uniqueIndex:uk_provider_attr;not null" json:"providerId"`
	AttrKey    string `gorm:"column:attr_key;type:varchar(64);uniqueIndex:uk_provider_attr;not null" json:"attrKey"`
	AttrType   string `gorm:"column:attr_type;type:varchar(16);not null" json:"attrType"`
	AttrValue  string `gorm:"column:attr_value;type:text" json:"attrValue"`
}

func (ProviderAttribute) TableName() string {
	return "provider_attributes"
}

// ProviderModel is one row of the normalized models table — replaces the
// default_models JSON blob. (provider_id, selection_id) is unique.
// CASCADE on provider_id ensures models vanish with their provider.
type ProviderModel struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ProviderID    uint64    `gorm:"column:provider_id;not null;uniqueIndex:uk_provider_model_selection;constraint:OnDelete:CASCADE" json:"providerId"`
	SelectionID   string    `gorm:"column:selection_id;type:varchar(128);not null;uniqueIndex:uk_provider_model_selection" json:"selectionId"`
	ModelID       string    `gorm:"column:model_id;type:varchar(128);not null;index:idx_provider_model_id" json:"modelId"`
	DisplayName   string    `gorm:"column:display_name;type:varchar(256)" json:"displayName"`
	ModelType     string    `gorm:"column:model_type;type:varchar(16);not null;index:idx_model_type" json:"modelType"`
	ContextWindow int       `gorm:"column:context_window" json:"contextWindow,omitempty"`
	Efforts       string    `gorm:"column:efforts;type:text" json:"-"`
	AigcCode      string    `gorm:"column:aigc_code;type:varchar(4);default:''" json:"aigcCode"`
	Status        string    `gorm:"column:status;type:varchar(8);default:'1'" json:"status"`
	SortOrder     int       `gorm:"column:sort_order;default:0" json:"sortOrder"`
	CreatedAt     time.Time `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt     time.Time `gorm:"column:updated_at;index" json:"updatedAt"`
}

func (ProviderModel) TableName() string { return "provider_models" }

// ── Repository abstraction ──────────────────────────────────────

// Repository is the high-level data-access contract for vendor presets.
// It is defined at the consumer boundary so the service depends on this
// interface, not on a concrete GORM repository.
type Repository interface {
	// Summary (primary table). 主表方法首参 tenantID：读路径 scope 到
	// 本租户 + 共享行（tenant_id=''，种子模板），写路径 scope 到本租户。
	ListAll(tenantID string) ([]*ProviderSummary, error)
	GetByID(tenantID string, id uint64) (*ProviderSummary, error)
	Create(tenantID string, p *ProviderSummary) error
	Update(tenantID string, p *ProviderSummary) error
	Delete(tenantID string, id uint64) error
	ExistsByKey(tenantID string, key string) (bool, error)
	// Count 是无租户上下文的系统路径：仅服务启动期 SeedIfEmpty（表全空
	// 才播种共享种子），跨租户全表计数。
	Count() (int64, error)
	// CopyForTenant copy-on-write：把共享模板（含子表行）复制为本租户行。
	CopyForTenant(tenantID string, srcID uint64) (*ProviderSummary, error)

	// Attributes (EAV). 子表不加 tenant 列，归属校验经主表
	// mustOwnProvider（同 Task 3 agents 关联表模式）。
	GetAttributes(tenantID string, providerID uint64) (map[string]AttrValue, error)
	SetAttributes(tenantID string, providerID uint64, attrs map[string]AttrValue) error

	// Models (new normalized child table)
	ListModels(tenantID string, providerID uint64) ([]ProviderModel, error)
	ListAllModels(tenantID string) ([]ProviderModel, error)
	GetModelBySelectionID(tenantID string, providerID uint64, selectionID string) (*ProviderModel, error)
	CreateModel(tenantID string, m *ProviderModel) error
	UpdateModel(tenantID string, m *ProviderModel) error
	DeleteModel(tenantID string, providerID uint64, selectionID string) error
	ReplaceModels(tenantID string, providerID uint64, models []ProviderModel) error
}
