package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type Provider interface {
	ID() uint64
	Key() string
	Name() string
	Description() string
	DescriptionEn() string
	Protocol() string
	AuthStyle() string
	BaseURL() string
	LockedAPIKey() string
	DefaultModels() []CatalogModel
	Fields() []PresetField
	IconKey() string
	Builtin() bool
	Attributes() map[string]AttrValue
	CreatedAt() time.Time
	UpdatedAt() time.Time
	Base() *BaseProvider

	// MultiRAGFactoryName returns the MultiRAG factory this provider syncs to
	// (e.g., "ZHIPU-AI", "MinerU"). Empty string means the provider has no
	// MultiRAG mapping; callers should treat the provider as not syncable.
	MultiRAGFactoryName() string

	// SyncToMultiRAG pushes this provider's configuration to a MultiRAG
	// instance via the given client. Each concrete provider implements
	// its own factory name and payload shape (per-model add_llm for LLM
	// providers, nested api_key add_llm for OCR providers). opts controls
	// verify-only mode.
	//
	// The default BaseProvider implementation returns ErrMultiRAGSyncNotImplemented.
	// Concrete providers override it.
	SyncToMultiRAG(ctx context.Context, client MultiRAGClient, opts SyncOptions) (*SyncResult, error)

	ToSummary() *ProviderSummary
}

type BaseProvider struct {
	id            uint64
	key           string
	name          string
	description   string
	descriptionEn string
	protocol      string
	authStyle     string
	baseURL       string
	lockedAPIKey  string
	defaultModels []CatalogModel
	fields        []PresetField
	iconKey       string
	builtin       bool
	attributes    map[string]AttrValue
	createdAt     time.Time
	updatedAt     time.Time
}

func (b *BaseProvider) ID() uint64                       { return b.id }
func (b *BaseProvider) Key() string                      { return b.key }
func (b *BaseProvider) Name() string                     { return b.name }
func (b *BaseProvider) Description() string              { return b.description }
func (b *BaseProvider) DescriptionEn() string            { return b.descriptionEn }
func (b *BaseProvider) Protocol() string                 { return b.protocol }
func (b *BaseProvider) AuthStyle() string                { return b.authStyle }
func (b *BaseProvider) BaseURL() string                  { return b.baseURL }
func (b *BaseProvider) LockedAPIKey() string             { return b.lockedAPIKey }
func (b *BaseProvider) DefaultModels() []CatalogModel    { return b.defaultModels }
func (b *BaseProvider) Fields() []PresetField            { return b.fields }
func (b *BaseProvider) IconKey() string                  { return b.iconKey }
func (b *BaseProvider) Builtin() bool                    { return b.builtin }
func (b *BaseProvider) Attributes() map[string]AttrValue { return b.attributes }
func (b *BaseProvider) CreatedAt() time.Time             { return b.createdAt }
func (b *BaseProvider) UpdatedAt() time.Time             { return b.updatedAt }

func (b *BaseProvider) Base() *BaseProvider { return b }

// MultiRAGFactoryName is the default no-op implementation. Concrete
// providers override this with their MultiRAG factory name.
func (b *BaseProvider) MultiRAGFactoryName() string { return "" }

// SyncToMultiRAG is the default no-op implementation. Concrete providers
// override this with their MultiRAG-specific sync logic.
func (b *BaseProvider) SyncToMultiRAG(ctx context.Context, client MultiRAGClient, opts SyncOptions) (*SyncResult, error) {
	return nil, ErrMultiRAGSyncNotImplemented
}

// syncAsAddLLM is the shared per-model sync implementation for LLM
// providers (Anthropic / OpenAI-API-Compatible factories). It issues one
// add_llm call per entry in defaultModels, carrying the provider's
// decrypted API key and base URL. A model whose ModelType cannot be mapped
// to a MultiRAG mdl_type is skipped and recorded as a failed call.
func (b *BaseProvider) syncAsAddLLM(ctx context.Context, client MultiRAGClient, opts SyncOptions, factoryName string) (*SyncResult, error) {
	res := &SyncResult{
		FactoryName: factoryName,
		Endpoint:    "add_llm",
		Success:     true,
	}
	models := b.defaultModels
	if len(models) == 0 {
		return res, nil
	}
	for _, m := range models {
		mdlType := MapModelTypeToMultiRAG(m.ModelType)
		if mdlType == "" {
			res.PerCall = append(res.PerCall, SyncCallResult{
				ModelName: m.ModelID,
				OK:        false,
				Error:     fmt.Sprintf("无法映射 model_type %q 到 MultiRAG（已知: llm, ocr, embedding, vlm）", m.ModelType),
			})
			res.Success = false
			continue
		}
		payload := AddLLMRequest{
			LLMFactory: factoryName,
			LLMName:    m.ModelID,
			MdlType:    mdlType,
			APIBase:    b.baseURL,
			MaxTokens:  m.ContextWindow,
			Verify:     opts.VerifyOnly,
		}
		if b.lockedAPIKey != "" {
			kb, err := json.Marshal(b.lockedAPIKey)
			if err != nil {
				return nil, fmt.Errorf("marshal api_key failed: %w", err)
			}
			payload.APIKey = kb
		}
		resp, err := client.AddLLM(ctx, payload)
		if err != nil {
			return nil, err
		}
		call := SyncCallResult{
			ModelName:  m.ModelID,
			HTTPStatus: resp.HTTPStatusCode,
			OK:         resp.Success,
			Error:      resp.Message,
			Raw:        resp.Raw,
		}
		if call.OK {
			call.Error = ""
		} else {
			res.Success = false
		}
		res.PerCall = append(res.PerCall, call)
	}
	res.CallCount = len(res.PerCall)
	return res, nil
}

// syncAsAddLLMWithNestedKey is the shared sync implementation for C 类
// providers whose api_key must be a nested object (MinerU, PaddleOCR).
// factoryName is the MultiRAG factory. nestedKey is the pre-built api_key
// object. extras carries any top-level extra fields (none for our current
// C 类 providers, but the hook is here for future Tencent/Bedrock/etc).
func (b *BaseProvider) syncAsAddLLMWithNestedKey(
	ctx context.Context,
	client MultiRAGClient,
	opts SyncOptions,
	factoryName string,
	nestedKey map[string]any,
	extras map[string]any,
) (*SyncResult, error) {
	models := b.defaultModels
	if len(models) == 0 {
		return nil, fmt.Errorf("C 类 provider %s 没有 models (no models to sync)", factoryName)
	}
	first := models[0]
	apiKeyJSON, err := json.Marshal(nestedKey)
	if err != nil {
		return nil, fmt.Errorf("marshal nested api_key 失败: %w", err)
	}
	payload := AddLLMRequest{
		LLMFactory: factoryName,
		LLMName:    first.ModelID,
		MdlType:    MapModelTypeToMultiRAG(first.ModelType),
		APIKey:     apiKeyJSON,
		APIBase:    "",
		MaxTokens:  0,
		Verify:     opts.VerifyOnly,
		Extras:     extras,
	}
	resp, err := client.AddLLM(ctx, payload)
	if err != nil {
		return nil, err
	}
	call := SyncCallResult{
		ModelName:  first.ModelID,
		HTTPStatus: resp.HTTPStatusCode,
		OK:         resp.Success,
		Error:      resp.Message,
		Raw:        resp.Raw,
	}
	if call.OK {
		call.Error = ""
	}
	return &SyncResult{
		FactoryName: factoryName,
		Endpoint:    "add_llm",
		CallCount:   1,
		Success:     call.OK,
		PerCall:     []SyncCallResult{call},
	}, nil
}

// SetSummary overwrites all base fields from a persisted summary.
// It is used by the service layer to populate a Provider before persistence.
// defaultModels is NOT loaded here; the service layer loads rows from the
// provider_models table and calls SetDefaultModels afterwards.
func (b *BaseProvider) SetSummary(s *ProviderSummary) error {
	if s == nil {
		return fmt.Errorf("provider summary is nil")
	}

	b.id = s.ID
	b.key = s.Key
	b.name = s.Name
	b.description = s.Description
	b.descriptionEn = s.DescriptionEn
	b.protocol = s.Protocol
	b.authStyle = s.AuthStyle
	b.baseURL = s.BaseURL
	b.lockedAPIKey = s.LockedAPIKey
	b.iconKey = s.IconKey
	b.builtin = s.Builtin
	b.createdAt = s.CreatedAt
	b.updatedAt = s.UpdatedAt

	b.defaultModels = nil

	b.fields = nil
	if s.Fields != "" {
		if err := json.Unmarshal([]byte(s.Fields), &b.fields); err != nil {
			return fmt.Errorf("unmarshal fields: %w", err)
		}
	}

	return nil
}

// SetDefaultModels injects models loaded from the provider_models table.
func (b *BaseProvider) SetDefaultModels(models []CatalogModel) { b.defaultModels = models }

// SetLockedAPIKey replaces the stored LockedAPIKey value. The service layer
// uses this to inject the decrypted plaintext key before calling
// SyncToMultiRAG, so concrete providers can read it via LockedAPIKey().
func (b *BaseProvider) SetLockedAPIKey(key string) { b.lockedAPIKey = key }

func (b *BaseProvider) ToSummary() *ProviderSummary {
	summary := &ProviderSummary{
		ID:            b.id,
		Key:           b.key,
		Name:          b.name,
		Description:   b.description,
		DescriptionEn: b.descriptionEn,
		Protocol:      b.protocol,
		AuthStyle:     b.authStyle,
		BaseURL:       b.baseURL,
		LockedAPIKey:  b.lockedAPIKey,
		IconKey:       b.iconKey,
		Builtin:       b.builtin,
		CreatedAt:     b.createdAt,
		UpdatedAt:     b.updatedAt,
	}

	// DefaultModels intentionally not written — provider_models table is the
	// source of truth (Task 4). Column is dropped in Task 7.

	if len(b.fields) == 0 {
		summary.Fields = "[]"
	} else if fieldsJSON, err := json.Marshal(b.fields); err == nil {
		summary.Fields = string(fieldsJSON)
	} else {
		summary.Fields = "[]"
	}

	return summary
}
