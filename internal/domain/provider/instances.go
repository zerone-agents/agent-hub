package provider

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
)

// ErrInstanceAPIUnavailable is only emitted by a read-only route probe.
// A mutation failure must never cause a fallback to legacy writes.
var ErrInstanceAPIUnavailable = errors.New("MultiRAG instance API unavailable")

type ModelRegistration struct {
	ModelName  string   `json:"model_name"`
	ModelTypes []string `json:"model_types"`
	MaxTokens  int      `json:"max_tokens"`
	IsTools    bool     `json:"is_tools"`
}

type InstanceCandidate struct {
	APIKey       string              `json:"api_key"`
	BaseURL      string              `json:"base_url"`
	Region       string              `json:"region"`
	ModelIDs     []string            `json:"model_ids"`
	CustomModels []ModelRegistration `json:"custom_models"`
}

type InstanceInput struct {
	InstanceCandidate
	InstanceName string `json:"instance_name"`
}

type RemoteInstance struct {
	ID           string `json:"id"`
	ProviderName string `json:"provider_name"`
	Name         string `json:"instance_name"`
	BaseURL      string `json:"base_url"`
	Region       string `json:"region"`
	Status       string `json:"status"`
	HasAPIKey    bool   `json:"has_api_key"`
}

type RemoteModel struct {
	ID              json.Number `json:"id"`
	Name            string      `json:"name"`
	ProviderName    string      `json:"provider_name"`
	InstanceID      string      `json:"instance_id"`
	InstanceName    string      `json:"instance_name"`
	ModelTypes      []string    `json:"model_types"`
	Status          string      `json:"status"`
	EffectiveStatus string      `json:"effective_status"`
	MaxTokens       int         `json:"max_tokens"`
	IsTools         bool        `json:"is_tools"`
	Reference       string      `json:"reference"`
}

func ValidModelReference(value string) bool {
	id, err := strconv.ParseInt(value, 10, 64)
	return err == nil && id > 0 && strconv.FormatInt(id, 10) == value
}

// MultiRAGInstances is used by the existing Hub sync, not by Desktop inference.
type MultiRAGInstances interface {
	SyncTarget() string
	ListInstances(context.Context, string) ([]RemoteInstance, error)
	VerifyInstance(context.Context, string, InstanceCandidate) error
	CreateInstance(context.Context, string, InstanceInput) (*RemoteInstance, error)
	UpdateInstance(context.Context, string, string, InstanceCandidate) (*RemoteInstance, error)
	ListInstanceModels(context.Context, string, string) ([]RemoteModel, error)
	AddInstanceModel(context.Context, string, string, ModelRegistration) (*RemoteModel, error)
}

// The random name is persisted BEFORE creation. An uncertain creation can only
// be reconciled by that exact name; it never authorizes another POST.
type MultiRAGSyncBinding struct {
	Version      uint64 `gorm:"not null;default:0"`
	Lease        string `gorm:"type:varchar(64);not null;default:''"`
	ProviderID   uint64 `gorm:"primaryKey;autoIncrement:false"`
	Target       string `gorm:"primaryKey;type:varchar(64)"`
	InstanceName string `gorm:"type:varchar(128);not null"`
	InstanceID   string `gorm:"type:varchar(64)"`
	ConfigHash   string `gorm:"type:varchar(64);not null"`
	State        string `gorm:"type:varchar(16);not null"`
}

func (MultiRAGSyncBinding) TableName() string { return "provider_multirag_bindings" }

type MultiRAGBindingRepository interface {
	GetMultiRAGBinding(string, uint64, string) (*MultiRAGSyncBinding, error)
	ReserveMultiRAGBinding(string, *MultiRAGSyncBinding) (bool, error)
	ConfirmMultiRAGBinding(string, *MultiRAGSyncBinding) error
	BeginMultiRAGSync(string, *MultiRAGSyncBinding) (bool, error)
	ReleaseMultiRAGBinding(string, *MultiRAGSyncBinding) error
}
