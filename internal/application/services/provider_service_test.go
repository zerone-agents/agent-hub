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
