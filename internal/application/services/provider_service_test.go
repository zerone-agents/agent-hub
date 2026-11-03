package services

import (
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"control-panel/internal/domain/provider"
	"control-panel/pkg/database"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedKeyCounter guarantees unique provider Keys across seedEmptyProvider
// calls. time.Now().UnixNano() is unsafe on Windows (clock resolution
// ~15ms), so two rapid calls can collide and violate UNIQUE(key).
var seedKeyCounter atomic.Uint64

const providerServiceTestEncryptionKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func setupProviderService(t *testing.T, apiKey string) *ProviderService {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderAttribute{}, &provider.ProviderModel{}))

	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })

	encrypted, err := provider.Encrypt(apiKey, providerServiceTestEncryptionKey)
	require.NoError(t, err)
	require.NoError(t, db.Create(&provider.ProviderSummary{
		ID:           1,
		Key:          "test-provider",
		Name:         "Test Provider",
		Protocol:     "openai",
		AuthStyle:    "api_key",
		LockedAPIKey: encrypted,
	}).Error)

	return NewProviderService(providerServiceTestEncryptionKey)
}

func setupEmptyProviderService(t *testing.T) *ProviderService {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderAttribute{}, &provider.ProviderModel{}))

	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })

	return NewProviderService(providerServiceTestEncryptionKey)
}

func TestProviderService_SeedIfEmpty(t *testing.T) {
	svc := setupEmptyProviderService(t)

	require.NoError(t, svc.SeedIfEmpty())

	summaries, err := svc.repo.ListAll("")
	require.NoError(t, err)
	require.Len(t, summaries, 7)

	keys := make(map[string]bool, len(summaries))
	var mineruSummary *provider.ProviderSummary
	for _, s := range summaries {
		keys[s.Key] = true
		if s.Key == "mineru" {
			mineruSummary = s
		}
	}
	require.True(t, keys["glm-cn"])
	require.True(t, keys["kimi-cn"])
	require.True(t, keys["bailian"])
	require.True(t, keys["anthropic-thirdparty"])
	require.True(t, keys["openai-thirdparty"])
	require.True(t, keys["mineru"])
	require.True(t, keys["paddleocr"])

	require.NotNil(t, mineruSummary)
	attrs, err := svc.repo.GetAttributes("", mineruSummary.ID)
	require.NoError(t, err)
	require.Equal(t, map[string]provider.AttrValue{
		"backend":       {Type: "string", Value: ""},
		"delete_output": {Type: "bool", Value: "false"},
		"output_dir":    {Type: "string", Value: ""},
	}, attrs)

	// GLM seed should have its models persisted to the provider_models table.
	var glmSummary *provider.ProviderSummary
	for _, s := range summaries {
		if s.Key == "glm-cn" {
			glmSummary = s
		}
	}
	require.NotNil(t, glmSummary)
	glmModels, err := svc.repo.ListModels("", glmSummary.ID)
	require.NoError(t, err)
	require.NotEmpty(t, glmModels, "glm-cn seed must persist models to provider_models")
	for _, m := range glmModels {
		require.Equal(t, "llm", m.ModelType, "glm-cn seed models must be tagged llm")
		require.NotEmpty(t, m.SelectionID, "seed models must have a selection_id")
	}

	// Idempotent
	require.NoError(t, svc.SeedIfEmpty())
	summaries, err = svc.repo.ListAll("")
	require.NoError(t, err)
	require.Len(t, summaries, 7)
}

func TestProviderServiceUpdate_PreservesKeyWhenMaskedValueIsSubmitted(t *testing.T) {
	svc := setupProviderService(t, "sk-original-secret-1234")
	maskedKey := "sk-o****1234"

	_, err := svc.Update("", 1, &UpdateProviderInput{LockedAPIKey: &maskedKey})
	require.NoError(t, err)

	storedKey, err := svc.GetRawAPIKey("", 1)
	require.NoError(t, err)
	require.Equal(t, "sk-original-secret-1234", storedKey)
}

func TestProviderServiceUpdate_ReplacesKeyWhenNewValueIsSubmitted(t *testing.T) {
	svc := setupProviderService(t, "sk-original-secret-1234")
	replacementKey := "sk-replacement-secret-5678"

	_, err := svc.Update("", 1, &UpdateProviderInput{LockedAPIKey: &replacementKey})
	require.NoError(t, err)

	storedKey, err := svc.GetRawAPIKey("", 1)
	require.NoError(t, err)
	require.Equal(t, replacementKey, storedKey)
}

// setupProviderServiceWithModels seeds a provider and two provider_models
// rows directly. The first row has an empty SelectionID to exercise the
// read-repair defense-in-depth path; the second has a stable SelectionID.
// (provider_id, selection_id) uniqueness permits one empty-string row
// alongside a populated one.
func setupProviderServiceWithModels(t *testing.T) *ProviderService {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderAttribute{}, &provider.ProviderModel{}))

	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })

	require.NoError(t, db.Create(&provider.ProviderSummary{
		ID:        1,
		Key:       "legacy-provider",
		Name:      "Legacy Provider",
		Protocol:  "openai",
		AuthStyle: "api_key",
		BaseURL:   "http://llm.example.com",
	}).Error)

	require.NoError(t, db.Create(&provider.ProviderModel{
		ProviderID: 1, SelectionID: "", ModelID: "gpt-4",
		DisplayName: "GPT-4", ModelType: "llm",
		ContextWindow: 8192, Status: "1", SortOrder: 0,
	}).Error)
	require.NoError(t, db.Create(&provider.ProviderModel{
		ProviderID: 1, SelectionID: "gpt-4-2", ModelID: "gpt-4",
		DisplayName: "GPT-4 (32K)", ModelType: "llm",
		ContextWindow: 32768, Status: "1", SortOrder: 1,
	}).Error)

	return NewProviderService(providerServiceTestEncryptionKey)
}

func TestProviderService_ReadRepair_BackfillsSelectionIDs(t *testing.T) {
	svc := setupProviderServiceWithModels(t)

	p, err := svc.GetByID("", 1)
	require.NoError(t, err)

	models := p.DefaultModels()
	require.Len(t, models, 2)
	require.Equal(t, "gpt-4", models[0].SelectionID)
	require.Equal(t, "gpt-4-2", models[1].SelectionID)

	// Read-repair persisted the new SelectionID to the row.
	rows, err := svc.repo.ListModels("", 1)
	require.NoError(t, err)
	require.Equal(t, "gpt-4", rows[0].SelectionID)
	require.Equal(t, "gpt-4-2", rows[1].SelectionID)

	// Idempotent: second read returns the same IDs.
	p2, err := svc.GetByID("", 1)
	require.NoError(t, err)
	require.Equal(t, models[0].SelectionID, p2.DefaultModels()[0].SelectionID)
	require.Equal(t, models[1].SelectionID, p2.DefaultModels()[1].SelectionID)
}

func TestProviderService_ReadRepair_ListAll(t *testing.T) {
	svc := setupProviderServiceWithModels(t)

	providers, err := svc.ListAll("", "")
	require.NoError(t, err)
	require.Len(t, providers, 1)
	for _, m := range providers[0].DefaultModels() {
		require.NotEmpty(t, m.SelectionID)
	}
}

func TestProviderService_Update_PreservesSelectionIDOnRename(t *testing.T) {
	svc := setupProviderServiceWithModels(t)

	// First, run read-repair so both rows have selection_ids.
	p, err := svc.GetByID("", 1)
	require.NoError(t, err)
	models := p.DefaultModels()
	require.Equal(t, "gpt-4", models[0].SelectionID)

	// Rename the display name only — selection_id must stay the same.
	renamed := models[0]
	renamed.DisplayName = "GPT-4 Renamed"
	newModels := []provider.CatalogModel{renamed, models[1]}
	_, err = svc.Update("", 1, &UpdateProviderInput{DefaultModels: &newModels})
	require.NoError(t, err)

	p2, err := svc.GetByID("", 1)
	require.NoError(t, err)
	require.Equal(t, "gpt-4", p2.DefaultModels()[0].SelectionID)
	require.Equal(t, "GPT-4 Renamed", p2.DefaultModels()[0].DisplayName)
	require.Equal(t, "gpt-4-2", p2.DefaultModels()[1].SelectionID)
}

func TestProviderService_Update_GeneratesSelectionIDForNewModels(t *testing.T) {
	svc := setupProviderServiceWithModels(t)

	// First, run read-repair so both rows have selection_ids.
	p, err := svc.GetByID("", 1)
	require.NoError(t, err)
	models := p.DefaultModels()

	// Append a third model with the same model_id and no explicit SelectionID;
	// EnsureSelectionIDs should allocate -3.
	newModels := append([]provider.CatalogModel{}, models...)
	newModels = append(newModels, provider.CatalogModel{
		ModelID: "gpt-4", DisplayName: "GPT-4 (128K)",
		ContextWindow: 131072, ModelType: "llm",
	})
	_, err = svc.Update("", 1, &UpdateProviderInput{DefaultModels: &newModels})
	require.NoError(t, err)

	p2, err := svc.GetByID("", 1)
	require.NoError(t, err)
	got := p2.DefaultModels()
	require.Len(t, got, 3)
	require.Equal(t, "gpt-4", got[0].SelectionID)
	require.Equal(t, "gpt-4-2", got[1].SelectionID)
	require.Equal(t, "gpt-4-3", got[2].SelectionID)
}

// ── New per-model CRUD methods (Task 4 Step 5) ──

func TestProviderService_AddModel_AppendsAndReturnsUpdatedDTO(t *testing.T) {
	svc := setupProviderServiceWithModels(t)

	// Baseline: 2 models after read-repair, sort_order 0 and 1.
	p, err := svc.GetByID("", 1)
	require.NoError(t, err)
	require.Len(t, p.DefaultModels(), 2)

	dto, err := svc.AddModel("", 1, &AddModelInput{
		ModelID:       "gpt-4o",
		DisplayName:   "GPT-4o",
		ModelType:     "llm",
		ContextWindow: 128000,
	})
	require.NoError(t, err)
	require.Len(t, dto.DefaultModels, 3)

	// The new model should appear with a selection_id matching its model_id.
	var found bool
	for _, m := range dto.DefaultModels {
		if m.ModelID == "gpt-4o" {
			found = true
			require.Equal(t, "gpt-4o", m.SelectionID)
			require.Equal(t, "llm", m.ModelType)
		}
	}
	require.True(t, found, "added model not present in DTO")

	// The appended model must be sorted AFTER the existing two. Without
	// setting SortOrder it would default to 0, tying with the first row
	// and surfacing in a surprising position.
	rows, err := svc.repo.ListModels("", 1)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	require.Equal(t, 0, rows[0].SortOrder)
	require.Equal(t, 1, rows[1].SortOrder)
	require.Equal(t, 2, rows[2].SortOrder, "appended model must sort after existing rows")
	require.Equal(t, "gpt-4o", rows[2].ModelID)
}

func TestProviderService_AddModel_RejectsUnknownModelType(t *testing.T) {
	svc := setupProviderServiceWithModels(t)

	_, err := svc.AddModel("", 1, &AddModelInput{
		ModelID:   "weird",
		ModelType: "bogus",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "type")
}

func TestProviderService_AddModel_RejectsEmptyModelID(t *testing.T) {
	svc := setupProviderServiceWithModels(t)

	_, err := svc.AddModel("", 1, &AddModelInput{
		ModelID:   "",
		ModelType: "llm",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "modelId")
}

func TestProviderService_AddModel_RejectsUnknownProvider(t *testing.T) {
	svc := setupProviderServiceWithModels(t)

	_, err := svc.AddModel("", 999, &AddModelInput{ModelID: "x", ModelType: "llm"})
	require.ErrorIs(t, err, provider.ErrProviderNotFound)
}

func TestProviderService_UpdateModel_PatchesFields(t *testing.T) {
	svc := setupProviderServiceWithModels(t)

	// Read-repair first to materialise "gpt-4" as the selection_id.
	_, err := svc.GetByID("", 1)
	require.NoError(t, err)

	newName := "GPT-4 (Renamed)"
	newContext := 65000
	dto, err := svc.UpdateModel("", 1, "gpt-4", &UpdateModelInput{
		DisplayName:   &newName,
		ContextWindow: &newContext,
	})
	require.NoError(t, err)

	var hit *provider.CatalogModel
	for i := range dto.DefaultModels {
		if dto.DefaultModels[i].SelectionID == "gpt-4" {
			hit = &dto.DefaultModels[i]
		}
	}
	require.NotNil(t, hit)
	require.Equal(t, newName, hit.DisplayName)
	require.Equal(t, newContext, hit.ContextWindow)
}

func TestProviderService_UpdateModel_RejectsUnknownModelType(t *testing.T) {
	svc := setupProviderServiceWithModels(t)

	// Run read-repair first so "gpt-4" exists as a selection_id.
	_, err := svc.GetByID("", 1)
	require.NoError(t, err)

	bad := "bogus"
	_, err = svc.UpdateModel("", 1, "gpt-4", &UpdateModelInput{ModelType: &bad})
	require.Error(t, err)
	require.Contains(t, err.Error(), "type")
}

func TestProviderService_UpdateModel_RejectsUnknownSelectionID(t *testing.T) {
	svc := setupProviderServiceWithModels(t)

	_, err := svc.UpdateModel("", 1, "ghost", &UpdateModelInput{})
	require.ErrorIs(t, err, provider.ErrProviderNotFound)
}

func TestProviderService_DeleteModel_RemovesRow(t *testing.T) {
	svc := setupProviderServiceWithModels(t)

	// Read-repair first.
	_, err := svc.GetByID("", 1)
	require.NoError(t, err)

	require.NoError(t, svc.DeleteModel("", 1, "gpt-4-2"))

	rows, err := svc.repo.ListModels("", 1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "gpt-4", rows[0].SelectionID)
}

func TestProviderService_DeleteModel_RejectsUnknownSelectionID(t *testing.T) {
	svc := setupProviderServiceWithModels(t)

	err := svc.DeleteModel("", 1, "ghost")
	require.ErrorIs(t, err, provider.ErrProviderNotFound)
}

func TestProviderService_GetByIDAsDTO_ReturnsFullDTO(t *testing.T) {
	svc := setupProviderServiceWithModels(t)

	// Read-repair first.
	_, err := svc.GetByID("", 1)
	require.NoError(t, err)

	dto, err := svc.GetByIDAsDTO("", 1)
	require.NoError(t, err)
	require.Equal(t, uint64(1), dto.ID)
	require.Len(t, dto.DefaultModels, 2)
}

func TestProviderService_GetByIDAsDTO_UnknownProvider(t *testing.T) {
	svc := setupProviderServiceWithModels(t)

	_, err := svc.GetByIDAsDTO("", 999)
	require.ErrorIs(t, err, provider.ErrProviderNotFound)
}

// ── Type filtering ──

func TestProviderService_ListAll_FiltersByModelType(t *testing.T) {
	svc := setupEmptyProviderService(t)

	// Create an LLM-only provider.
	_, err := svc.Create("", &CreateProviderInput{
		Key: "llm-only", Name: "LLM Only",
		Protocol: "openai", AuthStyle: "api_key",
		BaseURL: "http://llm.example.com",
		DefaultModels: []provider.CatalogModel{
			{ModelID: "gpt-4", DisplayName: "GPT-4", ModelType: "llm"},
		},
	})
	require.NoError(t, err)

	// Create a mixed provider with one LLM and one embedding model.
	_, err = svc.Create("", &CreateProviderInput{
		Key: "mixed", Name: "Mixed",
		Protocol: "openai", AuthStyle: "api_key",
		BaseURL: "http://mixed.example.com",
		DefaultModels: []provider.CatalogModel{
			{ModelID: "gpt-4", DisplayName: "GPT-4", ModelType: "llm"},
			{ModelID: "text-embedding-3", DisplayName: "Emb", ModelType: "embedding"},
		},
	})
	require.NoError(t, err)

	// ?type=embedding → only the mixed provider.
	emb, err := svc.ListAll("", "embedding")
	require.NoError(t, err)
	require.Len(t, emb, 1)
	require.Equal(t, "mixed", emb[0].Key())

	// ?type=llm → both providers.
	llm, err := svc.ListAll("", "llm")
	require.NoError(t, err)
	require.Len(t, llm, 2)

	// No filter → both.
	all, err := svc.ListAll("", "")
	require.NoError(t, err)
	require.Len(t, all, 2)
}

// TestProviderService_ListAll_ZeroModelChatCompatible 锁定 issue #240：零预置
// 模型的 openai/anthropic 兼容预设（模型全部由用户自带）必须在 chat 过滤下
// 保留（App「添加供应商」经 type=chat 加载预设），豁免仅限这两个 key 且仅限
// chat 伪类型——其他零模型 provider（含 OCR/embedding-only）照旧排除，不过滤
// 与严格类型过滤的行为不变。
func TestProviderService_ListAll_ZeroModelChatCompatible(t *testing.T) {
	svc := setupEmptyProviderService(t)

	zero := func(key, name string) {
		_, err := svc.Create("", &CreateProviderInput{
			Key: key, Name: name,
			Protocol: "openai", AuthStyle: "api_key",
			BaseURL: "http://" + key + ".example.com",
		})
		require.NoError(t, err)
	}
	// 两个豁免 key：零预置模型。
	zero("openai-thirdparty", "OpenAI Compatible")
	zero("anthropic-thirdparty", "Anthropic Compatible")
	// 非豁免 key 的零模型 provider：chat 下必须仍被排除。
	zero("other-zero", "Other Zero Model")
	// OCR-only 与 embedding-only：chat 下照旧排除（非 LLM/VLM）。
	_, err := svc.Create("", &CreateProviderInput{
		Key: "ocr-only", Name: "OCR Only",
		Protocol: "openai", AuthStyle: "api_key",
		BaseURL: "http://ocr.example.com",
		DefaultModels: []provider.CatalogModel{
			{ModelID: "mineru", DisplayName: "MinerU", ModelType: "ocr"},
		},
	})
	require.NoError(t, err)
	_, err = svc.Create("", &CreateProviderInput{
		Key: "emb-only", Name: "Embedding Only",
		Protocol: "openai", AuthStyle: "api_key",
		BaseURL: "http://emb.example.com",
		DefaultModels: []provider.CatalogModel{
			{ModelID: "text-embedding-3", DisplayName: "Emb", ModelType: "embedding"},
		},
	})
	require.NoError(t, err)

	// type=chat → 仅两个兼容预设（豁免），其余三零模型/非 LLM 全排除。
	chat, err := svc.ListAll("", "chat")
	require.NoError(t, err)
	require.Len(t, chat, 2)
	got := map[string]bool{}
	for _, p := range chat {
		got[p.Key()] = true
	}
	require.True(t, got["openai-thirdparty"], "openai-thirdparty must stay visible under chat filter")
	require.True(t, got["anthropic-thirdparty"], "anthropic-thirdparty must stay visible under chat filter")
	require.False(t, got["other-zero"], "non-exempt zero-model provider must stay excluded")
	require.False(t, got["ocr-only"], "OCR-only provider must stay excluded from chat")
	require.False(t, got["emb-only"], "embedding-only provider must stay excluded from chat")

	// 严格类型过滤行为不变：兼容预设零模型，llm 过滤下照旧排除。
	llm, err := svc.ListAll("", "llm")
	require.NoError(t, err)
	require.Len(t, llm, 0)

	// vlm 过滤与 llm 对称（issue #240 评审补充）：兼容预设零模型，照旧排除。
	vlm, err := svc.ListAll("", "vlm")
	require.NoError(t, err)
	require.Len(t, vlm, 0)

	// 其他类型过滤不变：ocr 过滤只中 ocr-only。
	ocr, err := svc.ListAll("", "ocr")
	require.NoError(t, err)
	require.Len(t, ocr, 1)
	require.Equal(t, "ocr-only", ocr[0].Key())

	// embedding 过滤与其他 type-filtered 行为对称：只中 emb-only。
	emb, err := svc.ListAll("", "embedding")
	require.NoError(t, err)
	require.Len(t, emb, 1)
	require.Equal(t, "emb-only", emb[0].Key())

	// 不过滤行为不变：全部 5 个可见。
	all, err := svc.ListAll("", "")
	require.NoError(t, err)
	require.Len(t, all, 5)
}

// TestProviderService_ListAll_ZeroModelChatCompatible_TenantCoW 锁定租户覆盖
// × chat 豁免组合（issue #240 评审补充；issue 正文要求 verify against
// tenant-specific provider overrides）：CoW 后同 key 存在共享行 + 租户行时，
// chat 过滤必须恰一条、且按「租户行胜出」后的存活行评估——零模型租户行走
// 豁免，加了 llm 模型的租户行走模型规则，共享行一律被遮蔽。
func TestProviderService_ListAll_ZeroModelChatCompatible_TenantCoW(t *testing.T) {
	svc := setupEmptyProviderService(t)
	require.NoError(t, svc.SeedIfEmpty())

	// 找 openai-thirdparty 共享行（种子零模型，评审亲验 seed_data.go）。
	summaries, err := svc.repo.ListAll("tenant-a")
	require.NoError(t, err)
	var shared *provider.ProviderSummary
	for _, s := range summaries {
		if s.Key == "openai-thirdparty" {
			shared = s
		}
	}
	require.NotNil(t, shared, "seed data must contain the openai-thirdparty shared row")
	require.Equal(t, "", shared.TenantID)

	// 场景①：租户 CoW 改名（租户行仍零模型）→ chat 恰一条、租户行胜出。
	newName := "tenant-a openai"
	cowDTO, err := svc.Update("tenant-a", shared.ID, &UpdateProviderInput{Name: &newName})
	require.NoError(t, err)
	require.NotEqual(t, shared.ID, cowDTO.ID, "CoW must produce a tenant-scoped copy row")

	chat, err := svc.ListAll("tenant-a", "chat")
	require.NoError(t, err)
	var hits []provider.Provider
	for _, p := range chat {
		if p.Key() == "openai-thirdparty" {
			hits = append(hits, p)
		}
	}
	require.Len(t, hits, 1, "CoW 后 chat 过滤必须恰一条（租户行遮蔽共享行）")
	require.Equal(t, cowDTO.ID, hits[0].ID(), "必须命中租户拷贝行而非共享模板行")

	// 场景②：租户行加 llm 模型 → chat 仍恰一条（命中模型规则），共享行仍被遮蔽。
	llmModels := []provider.CatalogModel{
		{ModelID: "custom-gpt", DisplayName: "Custom GPT", ModelType: "llm"},
	}
	_, err = svc.Update("tenant-a", cowDTO.ID, &UpdateProviderInput{DefaultModels: &llmModels})
	require.NoError(t, err)

	chat2, err := svc.ListAll("tenant-a", "chat")
	require.NoError(t, err)
	var hits2 []provider.Provider
	for _, p := range chat2 {
		if p.Key() == "openai-thirdparty" {
			hits2 = append(hits2, p)
		}
	}
	require.Len(t, hits2, 1, "加了 llm 模型的租户行 chat 下仍恰一条")
	require.Equal(t, cowDTO.ID, hits2[0].ID())

	// 租户行有 llm 模型后，llm 严格过滤下也恰一条——模型规则命中而非豁免
	// （豁免不泄漏到严格过滤器；种子其他 provider 不参与本 key 计数）。
	llmList, err := svc.ListAll("tenant-a", "llm")
	require.NoError(t, err)
	llmHits := 0
	for _, p := range llmList {
		if p.Key() == "openai-thirdparty" {
			llmHits++
		}
	}
	require.Equal(t, 1, llmHits, "租户行有 llm 模型后 llm 过滤下该 key 恰一条")
}

// TestZeroModelChatCompatibleKeysCoverSeedSpecs 把共编义务变成可执行门禁
// （issue #243 评审跟进）：BuiltinSeedSpecs 中零预置模型（DefaultModels 为空）
// 的预设 key 必须全部命中 chat 豁免集合（正向）；豁免集合也不得残留已改模型
// 形态或已删除的 key（反向）。任一方向漂移都会让预设错误地出现/消失在 chat
// 过滤列表，而纯注释约定无法阻止这种漂移。
func TestZeroModelChatCompatibleKeysCoverSeedSpecs(t *testing.T) {
	zeroModelKeys := map[string]bool{}
	for _, spec := range provider.BuiltinSeedSpecs() {
		if len(spec.DefaultModels) == 0 {
			zeroModelKeys[spec.Key] = true
		}
	}

	// 正向：零模型种子预设必须命中 chat 豁免集合，否则会被 chat 过滤隐藏。
	for key := range zeroModelKeys {
		require.True(t, zeroModelChatCompatibleKeys[key],
			"seed preset %q has zero preset models but is missing from zeroModelChatCompatibleKeys (issue #243 ①)", key)
	}

	// 反向：豁免集合中的 key 必须仍是零模型种子预设（防改名/改模型形态后残留）。
	for key := range zeroModelChatCompatibleKeys {
		require.True(t, zeroModelKeys[key],
			"zeroModelChatCompatibleKeys contains %q which is no longer a zero-model BuiltinSeedSpecs entry (issue #243 ①)", key)
	}

	// 豁免集合非空：防止集合被清空后正向断言空转、门禁失效。
	require.NotEmpty(t, zeroModelChatCompatibleKeys,
		"zeroModelChatCompatibleKeys must not be empty (issue #243 ①)")
}

func TestProviderService_Create_RejectsEmptyModelType(t *testing.T) {
	svc := setupEmptyProviderService(t)

	_, err := svc.Create("", &CreateProviderInput{
		Key: "bad", Name: "Bad",
		Protocol: "openai", AuthStyle: "api_key",
		BaseURL: "http://x.example.com",
		DefaultModels: []provider.CatalogModel{
			{ModelID: "gpt-4", DisplayName: "GPT-4"},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "modelType")
}

func TestProviderService_Update_RejectsEmptyModelType(t *testing.T) {
	svc := setupProviderServiceWithModels(t)

	// Read-repair first so rows have selection_ids.
	_, err := svc.GetByID("", 1)
	require.NoError(t, err)

	bad := []provider.CatalogModel{
		{ModelID: "gpt-4", DisplayName: "GPT-4"},
	}
	_, err = svc.Update("", 1, &UpdateProviderInput{DefaultModels: &bad})
	require.Error(t, err)
	require.Contains(t, err.Error(), "modelType")
}

// ── FindProviderByModelID (Task 2) ──

func TestProviderService_FindProviderByModelID(t *testing.T) {
	svc := setupProviderServiceWithModels(t)

	// Existing model_id — both seed rows share "gpt-4".
	p, err := svc.FindProviderByModelID("", "gpt-4")
	require.NoError(t, err)
	require.NotNil(t, p)
	require.Equal(t, "legacy-provider", p.Key())

	// Unknown model_id → sentinel error, nil provider.
	p, err = svc.FindProviderByModelID("", "does-not-exist")
	require.ErrorIs(t, err, provider.ErrProviderNotFound)
	require.Nil(t, p)

	// Empty model_id short-circuits to ErrProviderNotFound so we don't
	// accidentally return an arbitrary provider whose row happens to also
	// have an empty model_id (defense against degenerate matches).
	p, err = svc.FindProviderByModelID("", "")
	require.ErrorIs(t, err, provider.ErrProviderNotFound)
	require.Nil(t, p)
}

func TestModelEffortsConversionRoundTrip(t *testing.T) {
	models := []provider.CatalogModel{
		{SelectionID: "k3", ModelID: "k3", ModelType: "llm", Efforts: []string{"low", "high"}},
		{SelectionID: "k2", ModelID: "k2", ModelType: "llm"},
	}

	rows := toProviderModelRows(1, models)
	require.Equal(t, `["low","high"]`, rows[0].Efforts)
	require.Equal(t, "", rows[1].Efforts)

	back := toCatalogModels(rows)
	require.Equal(t, []string{"low", "high"}, back[0].Efforts)
	require.Nil(t, back[1].Efforts)
}

func TestEffortsFromJSON_InvalidReturnsNil(t *testing.T) {
	require.Nil(t, effortsFromJSON("not-json"))
	require.Nil(t, effortsFromJSON(""))
}

func TestProviderService_AddModel_WithEfforts(t *testing.T) {
	svc := setupProviderService(t, "sk-test")

	_, err := svc.AddModel("", 1, &AddModelInput{
		ModelID:   "k3",
		ModelType: "llm",
		Efforts:   []string{"low", "high"},
	})
	require.NoError(t, err)

	dto, err := svc.GetByIDAsDTO("", 1)
	require.NoError(t, err)
	require.Len(t, dto.DefaultModels, 1)
	require.Equal(t, []string{"low", "high"}, dto.DefaultModels[0].Efforts)
}

func TestProviderService_UpdateModel_Efforts(t *testing.T) {
	svc := setupProviderService(t, "sk-test")

	_, err := svc.AddModel("", 1, &AddModelInput{ModelID: "k3", ModelType: "llm"})
	require.NoError(t, err)

	efforts := []string{"max"}
	dto, err := svc.UpdateModel("", 1, "k3", &UpdateModelInput{Efforts: &efforts})
	require.NoError(t, err)
	require.Equal(t, []string{"max"}, dto.DefaultModels[0].Efforts)

	// nil = 不修改
	dto, err = svc.UpdateModel("", 1, "k3", &UpdateModelInput{})
	require.NoError(t, err)
	require.Equal(t, []string{"max"}, dto.DefaultModels[0].Efforts)

	// 空切片 = 清空
	empty := []string{}
	dto, err = svc.UpdateModel("", 1, "k3", &UpdateModelInput{Efforts: &empty})
	require.NoError(t, err)
	require.Empty(t, dto.DefaultModels[0].Efforts)
}

func TestAssignAigcCode_NewModelIncrements(t *testing.T) {
	existing := []provider.ProviderModel{
		{ModelID: "glm-4.5", AigcCode: "0001"},
		{ModelID: "gpt-4o", AigcCode: "0005"},
	}
	code, err := assignAigcCode("new-model", existing)
	require.NoError(t, err)
	require.Equal(t, "0006", code)
}

func TestAssignAigcCode_ReusesByModelID(t *testing.T) {
	existing := []provider.ProviderModel{
		{ModelID: "glm-4.5", AigcCode: "0001"},
		{ProviderID: 2, ModelID: "glm-4.5", AigcCode: "0001"}, // 跨 provider
		{ModelID: "gpt-4o", AigcCode: "0005"},
	}
	code, err := assignAigcCode("glm-4.5", existing)
	require.NoError(t, err)
	require.Equal(t, "0001", code)
}

func TestAssignAigcCode_EmptyExistingReturns0001(t *testing.T) {
	code, err := assignAigcCode("first-model", nil)
	require.NoError(t, err)
	require.Equal(t, "0001", code)
}

func TestAssignAigcCode_ZeroPads(t *testing.T) {
	existing := []provider.ProviderModel{{ModelID: "x", AigcCode: "0042"}}
	code, err := assignAigcCode("y", existing)
	require.NoError(t, err)
	require.Len(t, code, 4)
	require.Equal(t, "0043", code)
}

func TestAssignAigcCode_CapExceeded(t *testing.T) {
	existing := []provider.ProviderModel{{ModelID: "x", AigcCode: "9999"}}
	_, err := assignAigcCode("y", existing)
	require.Error(t, err)
	require.Contains(t, err.Error(), "槽位已满")
}

func TestAssignAigcCode_SkipsMalformedCodes(t *testing.T) {
	// 非 4 位 / 非数字的脏数据不影响递增计算
	existing := []provider.ProviderModel{
		{ModelID: "x", AigcCode: "ABC"},
		{ModelID: "y", AigcCode: "12"},
		{ModelID: "z", AigcCode: "0010"},
	}
	code, err := assignAigcCode("new", existing)
	require.NoError(t, err)
	require.Equal(t, "0011", code)
}

// ── AddModel × assignAigcCode integration (Task 3) ──

// setupProviderSvc spins up an in-memory sqlite DB with the provider schema
// migrated and returns a ProviderService backed by it. The global database.DB
// is swapped so repo code that falls back to the singleton keeps working.
func setupProviderSvc(t *testing.T) (*ProviderService, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderAttribute{}, &provider.ProviderModel{}))

	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })

	return NewProviderService(providerServiceTestEncryptionKey), db
}

// seedEmptyProvider inserts a bare provider row and returns its ID. Each call
// uses a unique Key (atomic counter + time) so multiple providers can coexist
// in one DB without colliding on unique constraints. The counter is required
// because time.Now().UnixNano() has ~15ms resolution on Windows and can
// produce identical timestamps for back-to-back calls.
func seedEmptyProvider(t *testing.T, db *gorm.DB) uint64 {
	t.Helper()
	seq := seedKeyCounter.Add(1)
	p := &provider.ProviderSummary{
		Key:       "p" + strconv.FormatUint(seq, 10) + "_" + strconv.FormatInt(time.Now().UnixNano(), 10),
		Name:      "test",
		Protocol:  "openai",
		AuthStyle: "api_key",
	}
	require.NoError(t, db.Create(p).Error)
	return p.ID
}

func TestAddModel_AssignsAigcCode(t *testing.T) {
	svc, db := setupProviderSvc(t)
	pid := seedEmptyProvider(t, db)

	dto, err := svc.AddModel("", pid, &AddModelInput{
		ModelID:     "glm-4.5",
		DisplayName: "GLM-4.5",
		ModelType:   "llm",
	})
	require.NoError(t, err)
	require.Len(t, dto.DefaultModels, 1)
	require.Equal(t, "0001", dto.DefaultModels[0].AigcCode)

	// 第二个不同模型递增
	dto2, err := svc.AddModel("", pid, &AddModelInput{
		ModelID:     "gpt-4o",
		DisplayName: "GPT-4o",
		ModelType:   "llm",
	})
	require.NoError(t, err)
	require.Equal(t, "0002", findModel(dto2.DefaultModels, "gpt-4o").AigcCode)
}

func TestAddModel_ReusesCodeForSameModelIDCrossProvider(t *testing.T) {
	svc, db := setupProviderSvc(t)
	pidA := seedEmptyProvider(t, db)
	pidB := seedEmptyProvider(t, db)

	_, err := svc.AddModel("", pidA, &AddModelInput{ModelID: "glm-4.5", ModelType: "llm"})
	require.NoError(t, err)
	dto, err := svc.AddModel("", pidB, &AddModelInput{ModelID: "glm-4.5", ModelType: "llm"})
	require.NoError(t, err)
	require.Equal(t, "0001", findModel(dto.DefaultModels, "glm-4.5").AigcCode)
}

// findModel 在 CatalogModel 列表中按 modelId 查找（测试辅助）
func findModel(ms []provider.CatalogModel, modelID string) provider.CatalogModel {
	for _, m := range ms {
		if m.ModelID == modelID {
			return m
		}
	}
	panic("model not found: " + modelID)
}

// ── bulk-save × assignAigcCode integration (Task 4) ──

func TestUpdate_PreservesExistingCodeAcrossEdit(t *testing.T) {
	svc, db := setupProviderSvc(t)
	pid := seedEmptyProvider(t, db)

	// 先添加一个模型，拿到码
	_, err := svc.AddModel("", pid, &AddModelInput{ModelID: "glm-4.5", ModelType: "llm"})
	require.NoError(t, err)

	// bulk Update：改 displayName 但保留 modelId → 码应稳定
	_, err = svc.Update("", pid, &UpdateProviderInput{
		DefaultModels: &[]provider.CatalogModel{
			{ModelID: "glm-4.5", DisplayName: "GLM 4.5 改名", ModelType: "llm"},
		},
	})
	require.NoError(t, err)

	dto, err := svc.GetByIDAsDTO("", pid)
	require.NoError(t, err)
	require.Equal(t, "0001", dto.DefaultModels[0].AigcCode)
}

func TestUpdate_AssignsNewCodeForNewModel(t *testing.T) {
	svc, db := setupProviderSvc(t)
	pid := seedEmptyProvider(t, db)

	_, err := svc.AddModel("", pid, &AddModelInput{ModelID: "glm-4.5", ModelType: "llm"})
	require.NoError(t, err)

	_, err = svc.Update("", pid, &UpdateProviderInput{
		DefaultModels: &[]provider.CatalogModel{
			{ModelID: "glm-4.5", ModelType: "llm"},
			{ModelID: "gpt-4o", ModelType: "llm"},
		},
	})
	require.NoError(t, err)

	dto, err := svc.GetByIDAsDTO("", pid)
	require.NoError(t, err)
	require.Equal(t, "0001", findModel(dto.DefaultModels, "glm-4.5").AigcCode)
	require.Equal(t, "0002", findModel(dto.DefaultModels, "gpt-4o").AigcCode)
}

func TestUpdate_DroppingModelDoesNotError(t *testing.T) {
	svc, db := setupProviderSvc(t)
	pid := seedEmptyProvider(t, db)

	_, err := svc.AddModel("", pid, &AddModelInput{ModelID: "glm-4.5", ModelType: "llm"})
	require.NoError(t, err)
	_, err = svc.AddModel("", pid, &AddModelInput{ModelID: "gpt-4o", ModelType: "llm"})
	require.NoError(t, err)

	// bulk 只保留 glm-4.5
	_, err = svc.Update("", pid, &UpdateProviderInput{
		DefaultModels: &[]provider.CatalogModel{
			{ModelID: "glm-4.5", ModelType: "llm"},
		},
	})
	require.NoError(t, err)
}

// ── Tenant isolation (multi-tenant Phase 3 Task 4) ──

// TestProviderService_CopyOnWrite_UpdateSharedProvider 租户修改共享种子
// provider 时：copy-on-write 复制为本租户行（ID 变化），共享模板原封不动，
// 其他租户仍看到模板原值。
func TestProviderService_CopyOnWrite_UpdateSharedProvider(t *testing.T) {
	svc := setupEmptyProviderService(t)
	require.NoError(t, svc.SeedIfEmpty())

	summaries, err := svc.repo.ListAll("tenant-a")
	require.NoError(t, err)
	require.NotEmpty(t, summaries)
	shared := summaries[0]
	require.Equal(t, "", shared.TenantID)

	newName := "tenant-a custom name"
	dto, err := svc.Update("tenant-a", shared.ID, &UpdateProviderInput{Name: &newName})
	require.NoError(t, err)
	require.NotEqual(t, shared.ID, dto.ID) // 复制为本租户行，ID 变化
	require.Equal(t, newName, dto.Name)

	// 共享模板未被改动
	orig, err := svc.repo.GetByID("tenant-b", shared.ID)
	require.NoError(t, err)
	require.Equal(t, shared.Name, orig.Name)
	require.Equal(t, "", orig.TenantID)

	// 租户 B 看不到租户 A 的定制行
	forTenantB, err := svc.repo.ListAll("tenant-b")
	require.NoError(t, err)
	for _, p := range forTenantB {
		require.NotEqual(t, dto.ID, p.ID)
	}
}

// TestProviderService_CopyOnWrite_ListAllDedupes CoW 后同 key 存在共享行 +
// 本租户拷贝行时，service 层 ListAll 必须只返回一行（本租户行遮蔽共享行）。
func TestProviderService_CopyOnWrite_ListAllDedupes(t *testing.T) {
	svc := setupEmptyProviderService(t)
	require.NoError(t, svc.SeedIfEmpty())

	// 找 glm-cn 共享行并 CoW 修改名称
	summaries, err := svc.repo.ListAll("tenant-a")
	require.NoError(t, err)
	var shared *provider.ProviderSummary
	for _, s := range summaries {
		if s.Key == "glm-cn" {
			shared = s
		}
	}
	require.NotNil(t, shared)
	require.Equal(t, "", shared.TenantID)

	newName := "tenant-a glm"
	dto, err := svc.Update("tenant-a", shared.ID, &UpdateProviderInput{Name: &newName})
	require.NoError(t, err)

	listed, err := svc.ListAll("tenant-a", "")
	require.NoError(t, err)
	seen := make(map[string]int)
	for _, p := range listed {
		seen[p.Key()]++
	}
	for key, n := range seen {
		require.Equal(t, 1, n, "key %s 出现 %d 次，CoW 后列表不得有同 key 双行", key, n)
	}

	// 遮蔽：glm-cn 命中的应是本租户定制行（新名称 + 新 ID）
	for _, p := range listed {
		if p.Key() == "glm-cn" {
			require.Equal(t, newName, p.Name())
			require.Equal(t, dto.ID, p.ID())
		}
	}
}

// TestProviderService_CopyOnWrite_FindProviderByModelIDTenantFirst CoW 后
// 双行存在时，FindProviderByModelID 必须返回本租户的定制配置（而非共享模板）。
func TestProviderService_CopyOnWrite_FindProviderByModelIDTenantFirst(t *testing.T) {
	svc := setupEmptyProviderService(t)
	require.NoError(t, svc.SeedIfEmpty())

	summaries, err := svc.repo.ListAll("tenant-a")
	require.NoError(t, err)
	var shared *provider.ProviderSummary
	for _, s := range summaries {
		if s.Key == "glm-cn" {
			shared = s
		}
	}
	require.NotNil(t, shared)

	// 挑 glm-cn 的一个 model_id，然后 CoW 改名
	models, err := svc.repo.ListModels("", shared.ID)
	require.NoError(t, err)
	require.NotEmpty(t, models)
	modelID := models[0].ModelID

	newName := "tenant-a glm"
	dto, err := svc.Update("tenant-a", shared.ID, &UpdateProviderInput{Name: &newName})
	require.NoError(t, err)

	p, err := svc.FindProviderByModelID("tenant-a", modelID)
	require.NoError(t, err)
	require.Equal(t, newName, p.Name())
	require.Equal(t, dto.ID, p.ID(), "必须命中本租户拷贝行，而非共享模板行")
}
