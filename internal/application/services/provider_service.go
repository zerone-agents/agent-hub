package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"control-panel/internal/domain/provider"
	repository "control-panel/internal/infrastructure/persistence"
)

// ProviderService handles CRUD for vendor presets, encryption, and probing.
// It depends on the provider.Repository interface, not a concrete repo,
// so the storage layout (summary + attributes tables) is swappable.
type ProviderService struct {
	repo          provider.Repository
	encryptionKey string
}

func NewProviderService(encryptionKey string) *ProviderService {
	return &ProviderService{
		repo:          repository.NewProviderRepository(),
		encryptionKey: encryptionKey,
	}
}

// ── DTO ─────────────────────────────────────────────────────────

type ProviderDTO struct {
	ID            uint64                        `json:"id"`
	Key           string                        `json:"key"`
	Name          string                        `json:"name"`
	Description   string                        `json:"description"`
	DescriptionEn string                        `json:"descriptionEn"`
	Protocol      string                        `json:"protocol"`
	AuthStyle     string                        `json:"authStyle"`
	BaseURL       string                        `json:"baseUrl"`
	DefaultModels []provider.CatalogModel       `json:"defaultModels"`
	Fields        []provider.PresetField        `json:"fields"`
	IconKey       string                        `json:"iconKey"`
	Builtin       bool                          `json:"builtin"`
	Attributes    map[string]provider.AttrValue `json:"attributes"`
	LockedAPIKey  string                        `json:"lockedApiKey"`
	CreatedAt     string                        `json:"createdAt"`
	UpdatedAt     string                        `json:"updatedAt"`
}

type ProbeResult struct {
	Success    bool   `json:"success"`
	LatencyMs  int64  `json:"latencyMs"`
	StatusCode int    `json:"statusCode"`
	Error      string `json:"error,omitempty"`
}

// ── Input structs ───────────────────────────────────────────────

type CreateProviderInput struct {
	Key           string
	Name          string
	Description   string
	DescriptionEn string
	Protocol      string
	AuthStyle     string
	BaseURL       string
	DefaultModels []provider.CatalogModel
	Fields        []provider.PresetField
	IconKey       string
	Builtin       bool
	Attributes    map[string]provider.AttrValue
	LockedAPIKey  string
}

type UpdateProviderInput struct {
	Name          *string
	Description   *string
	DescriptionEn *string
	Protocol      *string
	AuthStyle     *string
	BaseURL       *string
	DefaultModels *[]provider.CatalogModel
	Fields        *[]provider.PresetField
	IconKey       *string
	Builtin       *bool
	Attributes    map[string]provider.AttrValue
	LockedAPIKey  *string
}

// ── Seed ────────────────────────────────────────────────────────

// SeedIfEmpty inserts the initial vendor presets when the summary table
// has no rows. 种子 provider 走共享语义：写 tenant_id=”（全局共享模板），
// 所有租户经 TenantWithShared 读路径可见；租户修改共享模板时由
// ensureTenantOwned copy-on-write 复制为本租户行，不会改到模板本身。
func (s *ProviderService) SeedIfEmpty() error {
	count, err := s.repo.Count()
	if err != nil {
		return fmt.Errorf("检查 provider 表失败: %w", err)
	}
	if count > 0 {
		return nil
	}

	specs := provider.BuiltinSeedSpecs()
	inserted := 0
	for _, spec := range specs {
		preset := provider.NewFromSeedSpec(spec)

		summary := preset.ToSummary()
		if summary.LockedAPIKey != "" {
			encryptedKey, err := provider.Encrypt(summary.LockedAPIKey, s.encryptionKey)
			if err != nil {
				return fmt.Errorf("加密 LockedAPIKey 失败: %w", err)
			}
			summary.LockedAPIKey = encryptedKey
		}

		// Capture the preset's models BEFORE SetSummary resets the cache.
		presetModels := preset.DefaultModels()

		base := preset.Base()
		if err := s.repo.Create("", summary); err != nil {
			log.Printf("种子数据插入失败 (key=%s): %v", summary.Key, err)
			continue
		}
		if err := base.SetSummary(summary); err != nil {
			log.Printf("种子数据加载失败 (key=%s): %v", summary.Key, err)
			continue
		}

		// Persist the preset's default models to the provider_models table.
		if len(presetModels) > 0 {
			rows := toProviderModelRows(summary.ID, provider.EnsureSelectionIDs(presetModels))
			if err := s.repo.ReplaceModels("", summary.ID, rows); err != nil {
				log.Printf("种子模型插入失败 (key=%s): %v", summary.Key, err)
			} else {
				base.SetDefaultModels(provider.EnsureSelectionIDs(presetModels))
			}
		}

		// Seed attributes for protocols that declare them.
		if len(provider.ProviderAttrRules[summary.Protocol]) > 0 {
			attrs := make(map[string]provider.AttrValue)
			for _, rule := range provider.ProviderAttrRules[summary.Protocol] {
				attrs[rule.Key] = provider.AttrValue{Type: rule.Type, Value: rule.Default}
			}
			if err := s.repo.SetAttributes("", summary.ID, attrs); err != nil {
				log.Printf("种子属性插入失败 (key=%s): %v", summary.Key, err)
			}
		}
		inserted++
	}

	log.Printf("已插入 %d 条种子 provider 数据", inserted)
	return nil
}

// ensureTenantOwned 返回该租户可写的 provider 行：本租户行原样返回；
// 共享模板（tenant_id=”）先 copy-on-write 复制为本租户行再返回——注意
// 复制后 ID 会变，调用方必须使用返回行的 ID 做后续读写并把新 ID 透传给
// 前端（前端后续请求自然落在本租户行上）。他租户行返回 ErrRecordNotFound。
func (s *ProviderService) ensureTenantOwned(tenantID string, id uint64) (*provider.ProviderSummary, error) {
	summary, err := s.repo.GetByID(tenantID, id)
	if err != nil {
		return nil, err
	}
	if summary.TenantID == tenantID {
		return summary, nil
	}
	return s.repo.CopyForTenant(tenantID, id)
}

// ── Read ────────────────────────────────────────────────────────

// matchProviderType reports whether a model's type satisfies the requested
// filter. "chat" is a pseudo-type that matches both llm and vlm models, since
// VLM providers are usable as chat agents even when they have no pure-llm
// entries.
func matchProviderType(filter, modelType string) bool {
	if filter == "" {
		return true
	}
	if filter == "chat" {
		return modelType == "llm" || modelType == "vlm"
	}
	return filter == modelType
}

// ListAll returns all providers. When typeFilter is non-empty ("llm" or
// "ocr" or "embedding" or "vlm" — plus the pseudo-type "chat" which matches
// both llm and vlm), only providers that have at least one model of that
// type are returned.
func (s *ProviderService) ListAll(tenantID string, typeFilter string) ([]provider.Provider, error) {
	summaries, err := s.repo.ListAll(tenantID)
	if err != nil {
		return nil, fmt.Errorf("获取 Provider 列表失败: %w", err)
	}

	// Load all models once, group by provider_id. This avoids N+1 queries
	// when the provider list grows.
	allModels, err := s.repo.ListAllModels(tenantID)
	if err != nil {
		return nil, fmt.Errorf("加载 provider_models 失败: %w", err)
	}
	modelsByProvider := make(map[uint64][]provider.ProviderModel, len(summaries))
	providerHasType := make(map[uint64]bool)
	for _, m := range allModels {
		modelsByProvider[m.ProviderID] = append(modelsByProvider[m.ProviderID], m)
		if matchProviderType(typeFilter, m.ModelType) {
			providerHasType[m.ProviderID] = true
		}
	}

	result := make([]provider.Provider, 0, len(summaries))
	for _, summary := range summaries {
		if typeFilter != "" && !providerHasType[summary.ID] {
			continue
		}
		p, err := provider.NewProviderFromSummary(summary)
		if err != nil {
			return nil, err
		}
		// Set models as-loaded; repairSelectionIDs will fill any missing
		// SelectionIDs and persist the fix.
		p.Base().SetDefaultModels(toCatalogModels(modelsByProvider[summary.ID]))
		repaired, err := s.repairSelectionIDs(tenantID, summary.TenantID == tenantID, p)
		if err != nil {
			return nil, err
		}
		result = append(result, repaired)
	}
	return result, nil
}

// effortsToJSON serializes an effort list for the provider_models.efforts
// column. Empty/nil lists are stored as "".
func effortsToJSON(efforts []string) string {
	if len(efforts) == 0 {
		return ""
	}
	b, err := json.Marshal(efforts)
	if err != nil {
		return ""
	}
	return string(b)
}

// effortsFromJSON parses the provider_models.efforts column. Empty or
// malformed values yield nil (treated as "不涉及").
func effortsFromJSON(s string) []string {
	if s == "" {
		return nil
	}
	var efforts []string
	if err := json.Unmarshal([]byte(s), &efforts); err != nil {
		return nil
	}
	return efforts
}

// toCatalogModels converts ProviderModel rows to CatalogModel entries.
func toCatalogModels(rows []provider.ProviderModel) []provider.CatalogModel {
	out := make([]provider.CatalogModel, 0, len(rows))
	for _, r := range rows {
		out = append(out, provider.CatalogModel{
			SelectionID:   r.SelectionID,
			ModelID:       r.ModelID,
			DisplayName:   r.DisplayName,
			ContextWindow: r.ContextWindow,
			ModelType:     r.ModelType,
			Status:        r.Status,
			Efforts:       effortsFromJSON(r.Efforts),
			AigcCode:      r.AigcCode,
		})
	}
	return out
}

// toProviderModelRows converts CatalogModel entries into ProviderModel rows
// ready for persistence. SelectionID and ModelType must already be set on
// each CatalogModel (call EnsureSelectionIDs first).
func toProviderModelRows(providerID uint64, models []provider.CatalogModel) []provider.ProviderModel {
	rows := make([]provider.ProviderModel, 0, len(models))
	for i, m := range models {
		status := m.Status
		if status == "" {
			status = "1"
		}
		rows = append(rows, provider.ProviderModel{
			ProviderID:    providerID,
			SelectionID:   m.SelectionID,
			ModelID:       m.ModelID,
			DisplayName:   m.DisplayName,
			ModelType:     m.ModelType,
			ContextWindow: m.ContextWindow,
			Status:        status,
			SortOrder:     i,
			Efforts:       effortsToJSON(m.Efforts),
			AigcCode:      m.AigcCode,
		})
	}
	return rows
}

// repairSelectionIDs is a read-repair defense: if any model row has an
// empty SelectionID (e.g. legacy data that slipped past migration backfill
// and EnsureSelectionIDs), generate one and persist the full set via
// ReplaceModels. With the provider_models table this path is essentially
// dead — NOT NULL + unique constraint prevent empty/duplicate IDs at the
// storage layer — but kept as defense-in-depth. 共享模板（owned=false）只读，
// 跳过回写，避免读路径悄悄把模板复制/改写。
func (s *ProviderService) repairSelectionIDs(tenantID string, owned bool, p provider.Provider) (provider.Provider, error) {
	models := p.DefaultModels()
	if !provider.HasMissingSelectionIDs(models) {
		return p, nil
	}
	repaired := provider.EnsureSelectionIDs(models)
	p.Base().SetDefaultModels(repaired)
	if !owned {
		return p, nil
	}
	rows := toProviderModelRows(p.ID(), repaired)
	if err := s.repo.ReplaceModels(tenantID, p.ID(), rows); err != nil {
		return nil, fmt.Errorf("回写 selectionId 失败: %w", err)
	}
	return p, nil
}

// ProviderPresets has been removed: the frontend "preset selector" feature
// was deleted, and with it the only consumer of this endpoint. Seed data
// now lives in provider.BuiltinSeedSpecs().

func providerToDTO(p provider.Provider) *ProviderDTO {
	models := p.DefaultModels()
	if models == nil {
		models = []provider.CatalogModel{}
	}
	fields := p.Fields()
	if fields == nil {
		fields = []provider.PresetField{}
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	return &ProviderDTO{
		ID:            0,
		Key:           p.Key(),
		Name:          p.Name(),
		Description:   p.Description(),
		DescriptionEn: p.DescriptionEn(),
		Protocol:      p.Protocol(),
		AuthStyle:     p.AuthStyle(),
		BaseURL:       p.BaseURL(),
		DefaultModels: models,
		Fields:        fields,
		IconKey:       p.IconKey(),
		Builtin:       p.Builtin(),
		Attributes:    map[string]provider.AttrValue{},
		LockedAPIKey:  "",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func (s *ProviderService) GetByID(tenantID string, id uint64) (provider.Provider, error) {
	summary, err := s.repo.GetByID(tenantID, id)
	if err != nil {
		return nil, provider.ErrProviderNotFound
	}
	p, err := provider.NewProviderFromSummary(summary)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListModels(tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("加载 provider_models 失败: %w", err)
	}
	// Set models as-loaded; repairSelectionIDs will fill any missing
	// SelectionIDs and persist the fix.
	p.Base().SetDefaultModels(toCatalogModels(rows))
	return s.repairSelectionIDs(tenantID, summary.TenantID == tenantID, p)
}

// GetProviderByID is an alias for GetByID; it returns provider.ErrProviderNotFound
// when the DB row is missing.
func (s *ProviderService) GetProviderByID(tenantID string, id uint64) (provider.Provider, error) {
	return s.GetByID(tenantID, id)
}

// ── Create ──────────────────────────────────────────────────────

func (s *ProviderService) Create(tenantID string, input *CreateProviderInput) (*ProviderDTO, error) {
	if err := ValidateProviderKey(input.Key); err != nil {
		return nil, err
	}
	if input.Name == "" {
		return nil, fmt.Errorf("name 不能为空")
	}

	exists, err := s.repo.ExistsByKey(tenantID, input.Key)
	if err != nil {
		return nil, fmt.Errorf("检查 Provider 存在性失败: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("Provider '%s' 已存在", input.Key)
	}

	protocol := input.Protocol
	if protocol == "" {
		protocol = string(provider.ProtocolAnthropic)
	}
	authStyle := input.AuthStyle
	if authStyle == "" {
		authStyle = string(provider.AuthStyleAPIKey)
	}
	if err := validateProviderEnum(protocol, authStyle); err != nil {
		return nil, err
	}
	if err := provider.ValidateAttributes(protocol, input.Attributes); err != nil {
		return nil, err
	}
	if err := validateModels(input.DefaultModels); err != nil {
		return nil, err
	}

	p, err := provider.NewProvider(input.Key)
	if err != nil {
		p = provider.NewGenericProvider(protocol)
	}

	fieldsJSON, err := json.Marshal(input.Fields)
	if err != nil {
		return nil, fmt.Errorf("序列化 fields 失败: %w", err)
	}
	encryptedKey, err := provider.Encrypt(input.LockedAPIKey, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("加密 LockedAPIKey 失败: %w", err)
	}

	base := p.Base()
	if err := base.SetSummary(&provider.ProviderSummary{
		Key:           input.Key,
		Name:          input.Name,
		Description:   input.Description,
		DescriptionEn: input.DescriptionEn,
		Protocol:      protocol,
		AuthStyle:     authStyle,
		BaseURL:       input.BaseURL,
		Fields:        string(fieldsJSON),
		IconKey:       input.IconKey,
		Builtin:       input.Builtin,
		LockedAPIKey:  encryptedKey,
	}); err != nil {
		return nil, fmt.Errorf("加载 Provider 摘要失败: %w", err)
	}

	summary := p.ToSummary()
	if err := s.repo.Create(tenantID, summary); err != nil {
		return nil, fmt.Errorf("创建 Provider 失败: %w", err)
	}
	if err := base.SetSummary(summary); err != nil {
		return nil, fmt.Errorf("加载 Provider 摘要失败: %w", err)
	}

	// Persist default models to the provider_models table.
	if len(input.DefaultModels) > 0 {
		rows := toProviderModelRows(summary.ID, provider.EnsureSelectionIDs(input.DefaultModels))
		rows, err = s.assignBulkAigcCodes(tenantID, summary.ID, rows)
		if err != nil {
			return nil, err
		}
		if err := s.repo.ReplaceModels(tenantID, summary.ID, rows); err != nil {
			return nil, fmt.Errorf("写入 provider_models 失败: %w", err)
		}
	}

	// Load the persisted models back into the provider so ToDTO reflects them.
	rows, err := s.repo.ListModels(tenantID, summary.ID)
	if err != nil {
		return nil, fmt.Errorf("加载 provider_models 失败: %w", err)
	}
	base.SetDefaultModels(provider.EnsureSelectionIDs(toCatalogModels(rows)))

	if len(input.Attributes) > 0 {
		if err := s.repo.SetAttributes(tenantID, p.ID(), input.Attributes); err != nil {
			return nil, fmt.Errorf("写入 Provider 属性失败: %w", err)
		}
	}

	return s.ToDTO(tenantID, p)
}

// ── Update ──────────────────────────────────────────────────────

func (s *ProviderService) Update(tenantID string, id uint64, input *UpdateProviderInput) (*ProviderDTO, error) {
	// copy-on-write：目标是共享模板时先复制为本租户行（ID 随之变化），
	// 模板本身不被修改。
	summary, err := s.ensureTenantOwned(tenantID, id)
	if err != nil {
		return nil, provider.ErrProviderNotFound
	}
	id = summary.ID

	p, err := provider.NewProviderFromSummary(summary)
	if err != nil {
		return nil, err
	}

	// Resolve effective protocol/authStyle for validation of partial updates.
	effProtocol := derefStringDefault(input.Protocol, summary.Protocol)
	effAuthStyle := derefStringDefault(input.AuthStyle, summary.AuthStyle)
	if err := validateProviderEnum(effProtocol, effAuthStyle); err != nil {
		return nil, err
	}
	if input.Attributes != nil {
		if err := provider.ValidateAttributes(effProtocol, input.Attributes); err != nil {
			return nil, err
		}
	}

	if input.Name != nil {
		summary.Name = *input.Name
	}
	if input.Description != nil {
		summary.Description = *input.Description
	}
	if input.DescriptionEn != nil {
		summary.DescriptionEn = *input.DescriptionEn
	}
	if input.Protocol != nil {
		summary.Protocol = *input.Protocol
	}
	if input.AuthStyle != nil {
		summary.AuthStyle = *input.AuthStyle
	}
	if input.BaseURL != nil {
		summary.BaseURL = *input.BaseURL
	}
	if input.IconKey != nil {
		summary.IconKey = *input.IconKey
	}
	if input.Builtin != nil {
		summary.Builtin = *input.Builtin
	}
	if input.DefaultModels != nil {
		// Persist via the provider_models table.
		if err := validateModels(*input.DefaultModels); err != nil {
			return nil, err
		}
		models := provider.EnsureSelectionIDs(*input.DefaultModels)
		rows := toProviderModelRows(id, models)
		rows, err = s.assignBulkAigcCodes(tenantID, id, rows)
		if err != nil {
			return nil, err
		}
		if err := s.repo.ReplaceModels(tenantID, id, rows); err != nil {
			return nil, fmt.Errorf("替换 provider_models 失败: %w", err)
		}
	}
	if input.Fields != nil {
		fieldsJSON, err := json.Marshal(*input.Fields)
		if err != nil {
			return nil, fmt.Errorf("序列化 fields 失败: %w", err)
		}
		summary.Fields = string(fieldsJSON)
	}
	if input.LockedAPIKey != nil {
		shouldUpdate := true
		if *input.LockedAPIKey != "" && summary.LockedAPIKey != "" {
			storedKey, err := provider.Decrypt(summary.LockedAPIKey, s.encryptionKey)
			if err != nil {
				return nil, fmt.Errorf("解密 LockedAPIKey 失败: %w", err)
			}
			shouldUpdate = *input.LockedAPIKey != maskSecret(storedKey)
		}
		if shouldUpdate {
			encrypted, err := provider.Encrypt(*input.LockedAPIKey, s.encryptionKey)
			if err != nil {
				return nil, fmt.Errorf("加密 LockedAPIKey 失败: %w", err)
			}
			summary.LockedAPIKey = encrypted
		}
	}

	if err := s.repo.Update(tenantID, summary); err != nil {
		return nil, fmt.Errorf("更新 Provider 失败: %w", err)
	}
	if err := p.Base().SetSummary(summary); err != nil {
		return nil, fmt.Errorf("加载 Provider 摘要失败: %w", err)
	}

	// (Re)load the persisted models so ToDTO reflects the post-update state.
	rows, err := s.repo.ListModels(tenantID, p.ID())
	if err != nil {
		return nil, fmt.Errorf("加载 provider_models 失败: %w", err)
	}
	p.Base().SetDefaultModels(provider.EnsureSelectionIDs(toCatalogModels(rows)))

	if input.Attributes != nil {
		if err := s.repo.SetAttributes(tenantID, p.ID(), input.Attributes); err != nil {
			return nil, fmt.Errorf("更新 Provider 属性失败: %w", err)
		}
	}

	return s.ToDTO(tenantID, p)
}

// ── Delete ──────────────────────────────────────────────────────

func (s *ProviderService) Delete(tenantID string, id uint64) error {
	if _, err := s.repo.GetByID(tenantID, id); err != nil {
		return provider.ErrProviderNotFound
	}
	return s.repo.Delete(tenantID, id)
}

// ── Probe ───────────────────────────────────────────────────────

// ProbeWithOverride probes a provider. If apiKeyOverride matches the masked form of the
// stored key (or is empty), the real stored key is used; otherwise apiKeyOverride is used.
// baseURLOverride (when non-empty) replaces the stored BaseURL, so an edited-but-unsaved
// URL can be tested from the form. modelsOverride (when non-empty) replaces the stored
// model list for the same reason.
func (s *ProviderService) ProbeWithOverride(tenantID string, id uint64, apiKeyOverride, baseURLOverride string, modelsOverride []provider.CatalogModel) (*ProbeResult, error) {
	summary, err := s.repo.GetByID(tenantID, id)
	if err != nil {
		return nil, provider.ErrProviderNotFound
	}
	p, err := provider.NewProviderFromSummary(summary)
	if err != nil {
		return nil, err
	}

	storedKey, err := provider.Decrypt(p.LockedAPIKey(), s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("解密 LockedAPIKey 失败: %w", err)
	}

	apiKey := apiKeyOverride
	if apiKey == "" || apiKey == maskSecret(storedKey) {
		apiKey = storedKey
	}

	baseURL := p.BaseURL()
	if baseURLOverride != "" {
		baseURL = baseURLOverride
	}

	models := modelsOverride
	if len(models) == 0 {
		rows, err := s.repo.ListModels(tenantID, id)
		if err != nil {
			return nil, fmt.Errorf("加载 provider_models 失败: %w", err)
		}
		models = toCatalogModels(rows)
	}

	return s.doProbe(baseURL, apiKey, p.Protocol(), p.AuthStyle(), models), nil
}

func (s *ProviderService) ProbeConfig(baseURL, apiKey, protocol, authStyle string, models []provider.CatalogModel) *ProbeResult {
	return s.doProbe(baseURL, apiKey, protocol, authStyle, models)
}

func (s *ProviderService) doProbe(baseURL, apiKey, protocol, authStyle string, models []provider.CatalogModel) *ProbeResult {
	start := time.Now()
	base := strings.TrimSuffix(baseURL, "/")

	var req *http.Request
	var err error

	switch protocol {
	case string(provider.ProtocolOpenAI):
		req, err = http.NewRequest("GET", base+"/models", nil)
	case string(provider.ProtocolMinerU):
		req, err = http.NewRequest("GET", base+"/health", nil)
	case string(provider.ProtocolPaddleOCR):
		req, err = http.NewRequest("GET", base+"/health", nil)
	default:
		// Anthropic-compatible
		modelID := "claude-sonnet-4-20250514"
		if len(models) > 0 && models[0].ModelID != "" {
			modelID = models[0].ModelID
		}
		body := fmt.Sprintf(`{"model":"%s","max_tokens":1,"messages":[{"role":"user","content":"ping"}]}`, modelID)
		req, err = http.NewRequest("POST", base+"/v1/messages", strings.NewReader(body))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
		}
	}
	if err != nil {
		return &ProbeResult{Success: false, Error: err.Error()}
	}

	switch authStyle {
	case string(provider.AuthStyleAPIKey):
		req.Header.Set("x-api-key", apiKey)
	case string(provider.AuthStyleAuthToken):
		req.Header.Set("Authorization", "Bearer "+apiKey)
	case string(provider.AuthStyleNoAuth):
		// no auth header
	default:
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return &ProbeResult{Success: false, LatencyMs: latency, Error: err.Error()}
	}
	defer resp.Body.Close()

	success := resp.StatusCode >= 200 && resp.StatusCode < 400
	result := &ProbeResult{
		Success:    success,
		LatencyMs:  latency,
		StatusCode: resp.StatusCode,
	}
	if !success {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		result.Error = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return result
}

// ── Helpers ─────────────────────────────────────────────────────

func (s *ProviderService) ToDTO(tenantID string, p provider.Provider) (*ProviderDTO, error) {
	models := p.DefaultModels()
	if models == nil {
		models = []provider.CatalogModel{}
	}
	fields := p.Fields()
	if fields == nil {
		fields = []provider.PresetField{}
	}

	attributes, err := s.repo.GetAttributes(tenantID, p.ID())
	if err != nil {
		return nil, fmt.Errorf("读取 Provider 属性失败: %w", err)
	}
	if attributes == nil {
		attributes = map[string]provider.AttrValue{}
	}

	// Return masked API key (never expose plaintext)
	var maskedKey string
	if p.LockedAPIKey() != "" {
		raw, err := provider.Decrypt(p.LockedAPIKey(), s.encryptionKey)
		if err == nil {
			maskedKey = maskSecret(raw)
		}
	}

	return &ProviderDTO{
		ID:            p.ID(),
		Key:           p.Key(),
		Name:          p.Name(),
		Description:   p.Description(),
		DescriptionEn: p.DescriptionEn(),
		Protocol:      p.Protocol(),
		AuthStyle:     p.AuthStyle(),
		BaseURL:       p.BaseURL(),
		DefaultModels: models,
		Fields:        fields,
		IconKey:       p.IconKey(),
		Builtin:       p.Builtin(),
		Attributes:    attributes,
		LockedAPIKey:  maskedKey,
		CreatedAt:     p.CreatedAt().UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:     p.UpdatedAt().UTC().Format("2006-01-02T15:04:05Z"),
	}, nil
}

// validateProviderEnum rejects unknown protocol / authStyle values so the
// API cannot store arbitrary strings.
func validateProviderEnum(protocol, authStyle string) error {
	if protocol != "" && !validProtocols[protocol] {
		return fmt.Errorf("protocol 不支持: %s（可选: anthropic, openai, mineru）", protocol)
	}
	if authStyle != "" && !validAuthStyles[authStyle] {
		return fmt.Errorf("authStyle 不支持: %s（可选: api_key, auth_token, no_auth）", authStyle)
	}
	return nil
}

// validateProviderType rejects unknown provider type values.
func validateProviderType(t string) error {
	if t != "" && !validProviderTypes[t] {
		return fmt.Errorf("type 不支持: %s（可选: llm, ocr, embedding, vlm）", t)
	}
	return nil
}

// validateModels enforces that every CatalogModel has a non-empty ModelType
// drawn from the allowed set. The provider_models.model_type column is
// NOT NULL, so empty values cannot persist; this gives a clearer error
// than the resulting DB constraint violation.
func validateModels(models []provider.CatalogModel) error {
	for i, m := range models {
		if m.ModelType == "" {
			return fmt.Errorf("defaultModels[%d] (%s): modelType 不能为空", i, m.ModelID)
		}
		if err := validateProviderType(m.ModelType); err != nil {
			return fmt.Errorf("defaultModels[%d] (%s): %w", i, m.ModelID, err)
		}
	}
	return nil
}

var (
	validProtocols = map[string]bool{
		string(provider.ProtocolAnthropic): true,
		string(provider.ProtocolOpenAI):    true,
		string(provider.ProtocolMinerU):    true,
		string(provider.ProtocolPaddleOCR): true,
	}
	validAuthStyles = map[string]bool{
		string(provider.AuthStyleAPIKey):    true,
		string(provider.AuthStyleAuthToken): true,
		string(provider.AuthStyleNoAuth):    true,
	}
	validProviderTypes = map[string]bool{
		string(provider.TypeLLM):       true,
		string(provider.TypeOCR):       true,
		string(provider.TypeEmbedding): true,
		string(provider.TypeVLM):       true,
	}
)

// derefString returns the value of a *string, or "" when nil.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// derefStringDefault returns the *string value, or the fallback when nil.
func derefStringDefault(s *string, fallback string) string {
	if s == nil {
		return fallback
	}
	return *s
}

// maskSecret masks a secret value, showing only the first 4 and last 4 characters.
// Values with 8 or fewer characters are fully hidden.
func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= 8 {
		return "****"
	}
	return string(runes[:4]) + "****" + string(runes[len(runes)-4:])
}

// GetRawAPIKey returns the decrypted API key for a provider (internal use only).
func (s *ProviderService) GetRawAPIKey(tenantID string, id uint64) (string, error) {
	summary, err := s.repo.GetByID(tenantID, id)
	if err != nil {
		return "", provider.ErrProviderNotFound
	}
	p, err := provider.NewProviderFromSummary(summary)
	if err != nil {
		return "", err
	}
	if p.LockedAPIKey() == "" {
		return "", nil
	}
	return provider.Decrypt(p.LockedAPIKey(), s.encryptionKey)
}

// RevealAPIKey returns a provider's decrypted API key for the audited admin reveal endpoint.
func (s *ProviderService) RevealAPIKey(tenantID string, id uint64) (string, error) {
	summary, err := s.repo.GetByID(tenantID, id)
	if err != nil {
		return "", provider.ErrProviderNotFound
	}
	p, err := provider.NewProviderFromSummary(summary)
	if err != nil {
		return "", err
	}
	if p.LockedAPIKey() == "" {
		return "", nil
	}
	return provider.Decrypt(p.LockedAPIKey(), s.encryptionKey)
}

// ── Desktop runtime config ──────────────────────────────────────

// APIKeyStatus values returned by ListRuntimeConfigs so Desktop can tell
// "provider explicitly needs no key" apart from "key unreadable".
const (
	APIKeyStatusOK          = "ok"
	APIKeyStatusNone        = "none"
	APIKeyStatusUnavailable = "unavailable"
)

// ProviderRuntimeConfig is the per-provider runtime configuration served to
// authenticated Desktop clients. APIKey holds the plaintext key and must
// never be written to logs or caches.
type ProviderRuntimeConfig struct {
	ID           uint64 `json:"id"`
	Key          string `json:"key"`
	Name         string `json:"name"`
	Protocol     string `json:"protocol"`
	AuthStyle    string `json:"authStyle"`
	BaseURL      string `json:"baseUrl"`
	APIKey       string `json:"apiKey"`
	APIKeyStatus string `json:"apiKeyStatus"`
}

// ListRuntimeConfigs returns runtime configs for every provider, decrypting
// LockedAPIKey with the same logic as the admin reveal endpoint. A provider
// whose key fails to decrypt is reported with APIKeyStatusUnavailable instead
// of failing the whole batch.
func (s *ProviderService) ListRuntimeConfigs(tenantID string) ([]*ProviderRuntimeConfig, error) {
	summaries, err := s.repo.ListAll(tenantID)
	if err != nil {
		return nil, fmt.Errorf("获取 Provider 列表失败: %w", err)
	}

	configs := make([]*ProviderRuntimeConfig, 0, len(summaries))
	for _, summary := range summaries {
		p, err := provider.NewProviderFromSummary(summary)
		if err != nil {
			return nil, err
		}
		cfg := &ProviderRuntimeConfig{
			ID:        summary.ID,
			Key:       summary.Key,
			Name:      summary.Name,
			Protocol:  p.Protocol(),
			AuthStyle: p.AuthStyle(),
			BaseURL:   p.BaseURL(),
		}
		switch {
		case p.LockedAPIKey() == "":
			cfg.APIKeyStatus = APIKeyStatusNone
		default:
			plaintext, err := provider.Decrypt(p.LockedAPIKey(), s.encryptionKey)
			if err != nil {
				cfg.APIKeyStatus = APIKeyStatusUnavailable
			} else {
				cfg.APIKey = plaintext
				cfg.APIKeyStatus = APIKeyStatusOK
			}
		}
		configs = append(configs, cfg)
	}
	return configs, nil
}

// ── MultiRAG sync ───────────────────────────────────────────────

// SyncProviderToMultiRAG pushes a provider's configuration to MultiRAG.
// client is injected (constructed by the caller from config) so the
// service stays independent of transport wiring. verifyOnly controls
// MultiRAG's verify mode (validate-only vs. persist). modelIds optionally
// restricts which models to sync; when nil/empty, all models are synced.
//
// Returns provider.ErrMultiRAGConfigMissing when client is nil (server
// admin has not configured MultiRAG), or provider.ErrProviderNotFound
// when no row matches providerID.
func (s *ProviderService) SyncProviderToMultiRAG(ctx context.Context, tenantID string, providerID uint64, client provider.MultiRAGClient, verifyOnly bool, modelIds []string) (*provider.SyncResult, error) {
	if client == nil {
		return nil, provider.ErrMultiRAGConfigMissing
	}
	p, err := s.GetByID(tenantID, providerID)
	if err != nil {
		return nil, provider.ErrProviderNotFound
	}
	// Decrypt the stored API key so concrete providers can hand MultiRAG
	// plaintext credentials (LLM-class providers especially need this).
	if p.LockedAPIKey() != "" {
		plaintext, err := provider.Decrypt(p.LockedAPIKey(), s.encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("解密 LockedAPIKey 失败: %w", err)
		}
		p.Base().SetLockedAPIKey(plaintext)
	}
	if len(modelIds) > 0 {
		filtered := filterModels(p.DefaultModels(), modelIds)
		p.Base().SetDefaultModels(filtered)
	}
	return p.SyncToMultiRAG(ctx, client, provider.SyncOptions{VerifyOnly: verifyOnly})
}

// filterModels returns models whose ModelID is in the allowlist (case-sensitive).
func filterModels(models []provider.CatalogModel, ids []string) []provider.CatalogModel {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	out := make([]provider.CatalogModel, 0, len(ids))
	for _, m := range models {
		if set[m.ModelID] {
			out = append(out, m)
		}
	}
	return out
}

// ── Per-model CRUD (new in Task 4) ──────────────────────────────

// AddModelInput captures the fields needed to attach a new model row to
// an existing provider.
type AddModelInput struct {
	ModelID       string
	DisplayName   string
	ModelType     string
	ContextWindow int
	Efforts       []string
}

// UpdateModelInput captures the optional fields a PATCH on a single model
// row can change. Nil pointers are treated as "no change".
type UpdateModelInput struct {
	DisplayName   *string
	ModelType     *string
	ContextWindow *int
	Status        *string
	Efforts       *[]string
}

// GetByIDAsDTO is a convenience wrapper that loads a provider and assembles
// its DTO in one call.
func (s *ProviderService) GetByIDAsDTO(tenantID string, providerID uint64) (*ProviderDTO, error) {
	p, err := s.GetByID(tenantID, providerID)
	if err != nil {
		return nil, err
	}
	return s.ToDTO(tenantID, p)
}

// FindProviderByModelID returns the first provider whose DefaultModels
// contains a model with the given model_id. Used by KnowledgeService to
// translate local model_ids into MultiRAG-format full IDs (model@factory).
//
// Returns provider.ErrProviderNotFound when no provider matches, or when
// modelID is empty (short-circuit — never match providers whose rows
// happen to also have an empty model_id).
func (s *ProviderService) FindProviderByModelID(tenantID string, modelID string) (provider.Provider, error) {
	if modelID == "" {
		return nil, provider.ErrProviderNotFound
	}
	providers, err := s.ListAll(tenantID, "")
	if err != nil {
		return nil, err
	}
	for _, p := range providers {
		for _, m := range p.DefaultModels() {
			if m.ModelID == modelID {
				return p, nil
			}
		}
	}
	return nil, provider.ErrProviderNotFound
}

// AddModel appends a new model row to an existing provider and returns the
// updated provider DTO. 共享模板先 copy-on-write 复制为本租户行。
func (s *ProviderService) AddModel(tenantID string, providerID uint64, input *AddModelInput) (*ProviderDTO, error) {
	summary, err := s.ensureTenantOwned(tenantID, providerID)
	if err != nil {
		return nil, provider.ErrProviderNotFound
	}
	providerID = summary.ID
	if input.ModelID == "" {
		return nil, fmt.Errorf("modelId 不能为空")
	}
	if input.ModelType == "" {
		return nil, fmt.Errorf("modelType 不能为空")
	}
	if err := validateProviderType(input.ModelType); err != nil {
		return nil, err
	}

	// Compute the next sort_order so the appended model lands at the end
	// of the list. repo.ListModels returns rows sorted by sort_order ASC,
	// so the last row's SortOrder (if any) is the current maximum.
	existingRows, err := s.repo.ListModels(tenantID, providerID)
	if err != nil {
		return nil, fmt.Errorf("加载 provider_models 失败: %w", err)
	}
	maxSortOrder := 0
	for _, m := range existingRows {
		if m.SortOrder > maxSortOrder {
			maxSortOrder = m.SortOrder
		}
	}

	// Assign a stable AIGC code: reuse an existing code for the same model_id
	// across providers, otherwise allocate the next free 4-digit slot. The
	// snapshot must span all providers so cross-provider reuse works.
	allRows, err := s.repo.ListAllModels(tenantID)
	if err != nil {
		return nil, fmt.Errorf("加载全量 provider_models 失败: %w", err)
	}
	aigcCode, err := assignAigcCode(input.ModelID, allRows)
	if err != nil {
		return nil, err
	}

	row := &provider.ProviderModel{
		ProviderID:    providerID,
		SelectionID:   input.ModelID,
		ModelID:       input.ModelID,
		DisplayName:   input.DisplayName,
		ModelType:     input.ModelType,
		ContextWindow: input.ContextWindow,
		Status:        "1",
		SortOrder:     maxSortOrder + 1,
		Efforts:       effortsToJSON(input.Efforts),
		AigcCode:      aigcCode,
	}
	if err := s.repo.CreateModel(tenantID, row); err != nil {
		return nil, fmt.Errorf("创建 model 失败: %w", err)
	}
	return s.GetByIDAsDTO(tenantID, providerID)
}

// UpdateModel applies a partial update to a single model row identified by
// (providerID, selectionID) and returns the updated provider DTO. 共享模板
// 先 copy-on-write 复制为本租户行。
func (s *ProviderService) UpdateModel(tenantID string, providerID uint64, selectionID string, input *UpdateModelInput) (*ProviderDTO, error) {
	summary, err := s.ensureTenantOwned(tenantID, providerID)
	if err != nil {
		return nil, provider.ErrProviderNotFound
	}
	providerID = summary.ID
	row, err := s.repo.GetModelBySelectionID(tenantID, providerID, selectionID)
	if err != nil {
		return nil, provider.ErrProviderNotFound
	}
	if input.DisplayName != nil {
		row.DisplayName = *input.DisplayName
	}
	if input.ModelType != nil {
		if *input.ModelType == "" {
			return nil, fmt.Errorf("modelType 不能为空")
		}
		if err := validateProviderType(*input.ModelType); err != nil {
			return nil, err
		}
		row.ModelType = *input.ModelType
	}
	if input.ContextWindow != nil {
		row.ContextWindow = *input.ContextWindow
	}
	if input.Status != nil {
		row.Status = *input.Status
	}
	if input.Efforts != nil {
		row.Efforts = effortsToJSON(*input.Efforts)
	}
	if err := s.repo.UpdateModel(tenantID, row); err != nil {
		return nil, fmt.Errorf("更新 model 失败: %w", err)
	}
	return s.GetByIDAsDTO(tenantID, providerID)
}

// DeleteModel removes a single model row identified by (providerID, selectionID).
// 共享模板先 copy-on-write 复制为本租户行。
func (s *ProviderService) DeleteModel(tenantID string, providerID uint64, selectionID string) error {
	summary, err := s.ensureTenantOwned(tenantID, providerID)
	if err != nil {
		return provider.ErrProviderNotFound
	}
	providerID = summary.ID
	if _, err := s.repo.GetModelBySelectionID(tenantID, providerID, selectionID); err != nil {
		return provider.ErrProviderNotFound
	}
	return s.repo.DeleteModel(tenantID, providerID, selectionID)
}

// assignAigcCode resolves the AIGC model code for a model. If any existing
// row already has modelID with a non-empty code, that code is reused
// (cross-provider同名同码). Otherwise the next free 4-digit sequential
// code is assigned. Returns error when the 9999-slot capacity is exceeded.
func assignAigcCode(modelID string, existingRows []provider.ProviderModel) (string, error) {
	for _, r := range existingRows {
		if r.ModelID == modelID && r.AigcCode != "" {
			return r.AigcCode, nil
		}
	}
	max := 0
	for _, r := range existingRows {
		if len(r.AigcCode) != 4 {
			continue
		}
		n, err := strconv.Atoi(r.AigcCode)
		if err != nil {
			continue
		}
		if n > max {
			max = n
		}
	}
	next := max + 1
	if next > 9999 {
		return "", fmt.Errorf("AIGC 模型码槽位已满（最多 9999）")
	}
	return fmt.Sprintf("%04d", next), nil
}

// assignBulkAigcCodes fills AigcCode on each target row: codes for model_ids
// that already exist on this provider (matched by ModelID) are preserved
// across the wipe-and-reinsert; new model_ids get a fresh code. Cross-
// provider reuse is handled by assignAigcCode scanning allRows.
func (s *ProviderService) assignBulkAigcCodes(tenantID string, providerID uint64, rows []provider.ProviderModel) ([]provider.ProviderModel, error) {
	ownRows, err := s.repo.ListModels(tenantID, providerID)
	if err != nil {
		return nil, fmt.Errorf("加载现有 provider_models 失败: %w", err)
	}
	ownByModel := make(map[string]string, len(ownRows))
	for _, r := range ownRows {
		if r.AigcCode != "" {
			ownByModel[r.ModelID] = r.AigcCode
		}
	}
	allRows, err := s.repo.ListAllModels(tenantID)
	if err != nil {
		return nil, fmt.Errorf("加载全量 provider_models 失败: %w", err)
	}
	for i := range rows {
		if rows[i].AigcCode != "" {
			continue
		}
		if code, ok := ownByModel[rows[i].ModelID]; ok {
			rows[i].AigcCode = code
			continue
		}
		code, err := assignAigcCode(rows[i].ModelID, allRows)
		if err != nil {
			return nil, err
		}
		rows[i].AigcCode = code
		// Propagate so subsequent same-modelID rows in this batch reuse it.
		allRows = append(allRows, provider.ProviderModel{ModelID: rows[i].ModelID, AigcCode: code})
	}
	return rows, nil
}
