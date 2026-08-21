package services

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"control-panel/internal/domain/knowledge"
	"control-panel/internal/domain/provider"
	"control-panel/pkg/database"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type fakeKnowledgeEngine struct {
	retrievalFunc     func(ctx context.Context, req knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error)
	getDatasetFunc    func(ctx context.Context, id string) (*knowledge.Dataset, error)
	parseDocsFunc     func(ctx context.Context, datasetID string, ids []string) error
	listDatasetsFunc  func(ctx context.Context, req knowledge.DatasetListRequest) (*knowledge.DatasetListResult, error)
	downloadFunc      func(ctx context.Context, datasetID string, documentID string) (*knowledge.StreamResult, error)
	imageFunc         func(ctx context.Context, imageID string) (*knowledge.StreamResult, error)
	createDatasetFunc func(ctx context.Context, req knowledge.DatasetMutationRequest) (*knowledge.Dataset, error)
	updateDatasetFunc func(ctx context.Context, id string, req knowledge.DatasetMutationRequest) (*knowledge.Dataset, error)
}

func (f *fakeKnowledgeEngine) Health(ctx context.Context) (*knowledge.HealthStatus, error) {
	return &knowledge.HealthStatus{Configured: true, Connected: true, Status: "healthy"}, nil
}
func (f *fakeKnowledgeEngine) ListDatasets(ctx context.Context, req knowledge.DatasetListRequest) (*knowledge.DatasetListResult, error) {
	if f.listDatasetsFunc != nil {
		return f.listDatasetsFunc(ctx, req)
	}
	return &knowledge.DatasetListResult{}, nil
}
func (f *fakeKnowledgeEngine) CreateDataset(ctx context.Context, req knowledge.DatasetMutationRequest) (*knowledge.Dataset, error) {
	if f.createDatasetFunc != nil {
		return f.createDatasetFunc(ctx, req)
	}
	dataset := knowledge.Dataset(req)
	return &dataset, nil
}
func (f *fakeKnowledgeEngine) GetDataset(ctx context.Context, id string) (*knowledge.Dataset, error) {
	if f.getDatasetFunc != nil {
		return f.getDatasetFunc(ctx, id)
	}
	dataset := knowledge.Dataset{"id": id}
	return &dataset, nil
}
func (f *fakeKnowledgeEngine) UpdateDataset(ctx context.Context, id string, req knowledge.DatasetMutationRequest) (*knowledge.Dataset, error) {
	if f.updateDatasetFunc != nil {
		return f.updateDatasetFunc(ctx, id, req)
	}
	dataset := knowledge.Dataset(req)
	return &dataset, nil
}
func (f *fakeKnowledgeEngine) DeleteDatasets(ctx context.Context, req knowledge.DeleteRequest) error {
	return nil
}
func (f *fakeKnowledgeEngine) ListDocuments(ctx context.Context, datasetID string, req knowledge.DocumentListRequest) (*knowledge.DocumentListResult, error) {
	return &knowledge.DocumentListResult{}, nil
}
func (f *fakeKnowledgeEngine) UploadDocuments(ctx context.Context, datasetID string, upload knowledge.UploadRequest) ([]knowledge.Document, error) {
	return []knowledge.Document{{"id": "doc1"}}, nil
}
func (f *fakeKnowledgeEngine) DownloadDocument(ctx context.Context, datasetID string, documentID string) (*knowledge.StreamResult, error) {
	if f.downloadFunc != nil {
		return f.downloadFunc(ctx, datasetID, documentID)
	}
	return &knowledge.StreamResult{Body: io.NopCloser(strings.NewReader("doc"))}, nil
}
func (f *fakeKnowledgeEngine) GetImage(ctx context.Context, imageID string) (*knowledge.StreamResult, error) {
	if f.imageFunc != nil {
		return f.imageFunc(ctx, imageID)
	}
	return &knowledge.StreamResult{Body: io.NopCloser(strings.NewReader("image"))}, nil
}
func (f *fakeKnowledgeEngine) UpdateDocument(ctx context.Context, datasetID string, documentID string, req knowledge.DocumentUpdateRequest) (*knowledge.Document, error) {
	document := knowledge.Document(req)
	return &document, nil
}
func (f *fakeKnowledgeEngine) DeleteDocuments(ctx context.Context, datasetID string, req knowledge.DeleteRequest) error {
	return nil
}
func (f *fakeKnowledgeEngine) ParseDocuments(ctx context.Context, datasetID string, ids []string) error {
	if f.parseDocsFunc != nil {
		return f.parseDocsFunc(ctx, datasetID, ids)
	}
	return nil
}
func (f *fakeKnowledgeEngine) StopParsingDocuments(ctx context.Context, datasetID string, ids []string) error {
	return nil
}
func (f *fakeKnowledgeEngine) ListChunks(ctx context.Context, datasetID string, documentID string, req knowledge.ChunkListRequest) (*knowledge.ChunkListResult, error) {
	return &knowledge.ChunkListResult{}, nil
}
func (f *fakeKnowledgeEngine) CreateChunk(ctx context.Context, datasetID string, documentID string, req knowledge.ChunkMutationRequest) (*knowledge.Chunk, error) {
	chunk := knowledge.Chunk(req)
	return &chunk, nil
}
func (f *fakeKnowledgeEngine) UpdateChunk(ctx context.Context, datasetID string, documentID string, chunkID string, req knowledge.ChunkMutationRequest) (*knowledge.Chunk, error) {
	chunk := knowledge.Chunk(req)
	return &chunk, nil
}
func (f *fakeKnowledgeEngine) DeleteChunks(ctx context.Context, datasetID string, documentID string, req knowledge.DeleteChunksRequest) error {
	return nil
}
func (f *fakeKnowledgeEngine) SwitchChunks(ctx context.Context, datasetID string, documentID string, ids []string, available bool) error {
	return nil
}
func (f *fakeKnowledgeEngine) Retrieval(ctx context.Context, req knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error) {
	if f.retrievalFunc != nil {
		return f.retrievalFunc(ctx, req)
	}
	result := knowledge.RetrievalResult{"ok": true}
	return &result, nil
}

func TestKnowledgeService_MissingConfig(t *testing.T) {
	svc := NewKnowledgeService(nil, nil)
	_, err := svc.ListDatasets(context.Background(), knowledge.DatasetListRequest{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if knowledge.StatusCode(err) != 503 {
		t.Fatalf("StatusCode = %d, want 503", knowledge.StatusCode(err))
	}
	status, healthErr := svc.Health(context.Background())
	if healthErr == nil {
		t.Fatal("expected health error, got nil")
	}
	if status.Configured || status.Connected || status.Status != "unavailable" {
		t.Fatalf("unexpected health status: %#v", status)
	}
}

func TestKnowledgeService_RetrievalUsesDatasetIDs(t *testing.T) {
	var got knowledge.RetrievalRequest
	svc := NewKnowledgeService(&fakeKnowledgeEngine{
		retrievalFunc: func(ctx context.Context, req knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error) {
			got = req
			result := knowledge.RetrievalResult{"total": 0}
			return &result, nil
		},
	}, nil)
	_, err := svc.Retrieval(context.Background(), knowledge.RetrievalRequest{
		"question": "hello",
		"kb_ids":   []string{"kb1"},
		"doc_ids":  []string{"doc1"},
	})
	if err != nil {
		t.Fatalf("Retrieval failed: %v", err)
	}
	if _, ok := got["kb_ids"]; ok {
		t.Fatalf("kb_ids leaked into engine request: %#v", got)
	}
	if _, ok := got["doc_ids"]; ok {
		t.Fatalf("doc_ids leaked into engine request: %#v", got)
	}
	if got["dataset_ids"] == nil || got["document_ids"] == nil {
		t.Fatalf("expected dataset_ids/document_ids, got %#v", got)
	}
}

func TestKnowledgeService_TrimsIDsForParse(t *testing.T) {
	var gotDatasetID string
	var gotIDs []string
	svc := NewKnowledgeService(&fakeKnowledgeEngine{
		parseDocsFunc: func(ctx context.Context, datasetID string, ids []string) error {
			gotDatasetID = datasetID
			gotIDs = ids
			return nil
		},
	}, nil)
	if err := svc.ParseDocuments(context.Background(), " kb1 ", []string{" doc1 ", "", "doc2"}); err != nil {
		t.Fatalf("ParseDocuments failed: %v", err)
	}
	if gotDatasetID != "kb1" {
		t.Fatalf("datasetID = %q, want kb1", gotDatasetID)
	}
	if len(gotIDs) != 2 || gotIDs[0] != "doc1" || gotIDs[1] != "doc2" {
		t.Fatalf("ids = %#v, want doc1/doc2", gotIDs)
	}
}

func TestKnowledgeService_GetDatasetRequiresID(t *testing.T) {
	svc := NewKnowledgeService(&fakeKnowledgeEngine{
		getDatasetFunc: func(ctx context.Context, id string) (*knowledge.Dataset, error) {
			return nil, errors.New("should not be called")
		},
	}, nil)
	_, err := svc.GetDataset(context.Background(), " ")
	if err == nil || knowledge.StatusCode(err) != 400 {
		t.Fatalf("expected bad request, got %v", err)
	}
}

func TestKnowledgeService_DownloadRequiresIDsAndDelegates(t *testing.T) {
	var gotDatasetID string
	var gotDocumentID string
	svc := NewKnowledgeService(&fakeKnowledgeEngine{
		downloadFunc: func(ctx context.Context, datasetID string, documentID string) (*knowledge.StreamResult, error) {
			gotDatasetID = datasetID
			gotDocumentID = documentID
			return &knowledge.StreamResult{Body: io.NopCloser(strings.NewReader("doc"))}, nil
		},
	}, nil)
	stream, err := svc.DownloadDocument(context.Background(), " kb1 ", " doc1 ")
	if err != nil {
		t.Fatalf("DownloadDocument failed: %v", err)
	}
	defer stream.Body.Close()
	if gotDatasetID != "kb1" || gotDocumentID != "doc1" {
		t.Fatalf("delegation failed: dataset=%q document=%q", gotDatasetID, gotDocumentID)
	}
}

func TestKnowledgeService_GetImageRequiresDatasetAndImageID(t *testing.T) {
	var gotImageID string
	svc := NewKnowledgeService(&fakeKnowledgeEngine{
		imageFunc: func(ctx context.Context, imageID string) (*knowledge.StreamResult, error) {
			gotImageID = imageID
			return &knowledge.StreamResult{Body: io.NopCloser(strings.NewReader("img"))}, nil
		},
	}, nil)
	stream, err := svc.GetImage(context.Background(), " kb1 ", " img1 ")
	if err != nil {
		t.Fatalf("GetImage failed: %v", err)
	}
	defer stream.Body.Close()
	if gotImageID != "img1" {
		t.Fatalf("imageID = %q, want img1", gotImageID)
	}
}

// ── Task 4: local model_id → MultiRAG translation ───────────────

// setupKnowledgeProviderSvc seeds an in-memory DB with two providers and
// their models so KnowledgeService translation tests can run without a
// live MultiRAG or Postgres. Returns a ProviderService whose
// FindProviderByModelID will resolve:
//   - "bge-large-zh" → GLM-branded anthropic provider (factory "Anthropic"
//     after the LLM consolidation; previously "ZHIPU-AI")
//   - "mineru"       → MinerU (factory "MinerU")
func setupKnowledgeProviderSvc(t *testing.T) *ProviderService {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderAttribute{}, &provider.ProviderModel{}))

	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })

	// GLM-branded anthropic provider with an embedding model "bge-large-zh".
	// Constructed via NewFromSeedSpec so we don't depend on the deleted
	// glm.go type.
	glm := provider.NewFromSeedSpec(provider.SeedSpec{
		Key:       "glm-cn",
		Name:      "GLM Coding Plan",
		Protocol:  string(provider.ProtocolAnthropic),
		AuthStyle: string(provider.AuthStyleAPIKey),
		BaseURL:   "https://open.bigmodel.cn/api/anthropic",
	})
	glmSummary := glm.ToSummary()
	require.NoError(t, db.Create(glmSummary).Error)
	require.NoError(t, db.Create(&provider.ProviderModel{
		ProviderID: glmSummary.ID, SelectionID: "bge-large-zh",
		ModelID: "bge-large-zh", DisplayName: "BGE Large ZH",
		ModelType: string(provider.TypeEmbedding), Status: "1", SortOrder: 0,
	}).Error)

	// MinerU with model "mineru" — factory is "MinerU".
	mineru := provider.NewMinerU()
	mineruSummary := mineru.ToSummary()
	require.NoError(t, db.Create(mineruSummary).Error)
	require.NoError(t, db.Create(&provider.ProviderModel{
		ProviderID: mineruSummary.ID, SelectionID: "mineru",
		ModelID: "mineru", DisplayName: "MinerU",
		ModelType: string(provider.TypeOCR), Status: "1", SortOrder: 0,
	}).Error)

	return NewProviderService(providerServiceTestEncryptionKey)
}

// TestKnowledgeService_TranslatesLocalEmbdID verifies that when a request
// arrives with a local model_id ("bge-large-zh"), the service rewrites it
// to the MultiRAG full-ID form ("<model_id>@<factory>") before forwarding.
func TestKnowledgeService_TranslatesLocalEmbdID(t *testing.T) {
	var captured knowledge.DatasetMutationRequest
	engine := &fakeKnowledgeEngine{
		createDatasetFunc: func(ctx context.Context, req knowledge.DatasetMutationRequest) (*knowledge.Dataset, error) {
			captured = req
			dataset := knowledge.Dataset(req)
			return &dataset, nil
		},
	}
	svc := NewKnowledgeService(engine, setupKnowledgeProviderSvc(t))

	_, err := svc.CreateDataset(context.Background(), "default", knowledge.DatasetMutationRequest{
		"embd_id": "bge-large-zh",
	})
	require.NoError(t, err)

	// After LLM consolidation, GLM-branded providers sync under the generic
	// "Anthropic" factory name (was "ZHIPU-AI" before glm.go was removed).
	require.Equal(t, "bge-large-zh@Anthropic", captured["embd_id"])
}

// TestKnowledgeService_PassThroughMultiRAGEmbdID verifies that an
// already-MultiRAG-formatted embedding id ("bge-m3@BAAI") is forwarded
// unchanged because no local provider_models row has that model_id.
func TestKnowledgeService_PassThroughMultiRAGEmbdID(t *testing.T) {
	var captured knowledge.DatasetMutationRequest
	engine := &fakeKnowledgeEngine{
		createDatasetFunc: func(ctx context.Context, req knowledge.DatasetMutationRequest) (*knowledge.Dataset, error) {
			captured = req
			dataset := knowledge.Dataset(req)
			return &dataset, nil
		},
	}
	svc := NewKnowledgeService(engine, setupKnowledgeProviderSvc(t))

	_, err := svc.CreateDataset(context.Background(), "default", knowledge.DatasetMutationRequest{
		"embd_id": "bge-m3@BAAI",
	})
	require.NoError(t, err)

	require.Equal(t, "bge-m3@BAAI", captured["embd_id"])
}

// TestKnowledgeService_TranslatesLocalLayout verifies that a local
// layout_recognize value matching a provider model_id ("mineru") is
// translated to the factory name only ("MinerU"), as MultiRAG's
// layout_recognize uses factory names rather than full IDs.
func TestKnowledgeService_TranslatesLocalLayout(t *testing.T) {
	var captured knowledge.DatasetMutationRequest
	engine := &fakeKnowledgeEngine{
		updateDatasetFunc: func(ctx context.Context, id string, req knowledge.DatasetMutationRequest) (*knowledge.Dataset, error) {
			captured = req
			dataset := knowledge.Dataset(req)
			return &dataset, nil
		},
	}
	svc := NewKnowledgeService(engine, setupKnowledgeProviderSvc(t))

	_, err := svc.UpdateDataset(context.Background(), "default", "kb1", knowledge.DatasetMutationRequest{
		"parser_config": map[string]any{
			"layout_recognize": "mineru",
		},
	})
	require.NoError(t, err)

	parserConfig, ok := captured["parser_config"].(map[string]any)
	require.True(t, ok, "parser_config must remain a map[string]any after translation; got %T", captured["parser_config"])
	require.Equal(t, "MinerU", parserConfig["layout_recognize"])
}

// TestKnowledgeService_PassThroughBuiltinLayout verifies that a builtin
// layout_recognize value ("DeepDOC") that does not match any local
// provider model_id is forwarded unchanged.
func TestKnowledgeService_PassThroughBuiltinLayout(t *testing.T) {
	var captured knowledge.DatasetMutationRequest
	engine := &fakeKnowledgeEngine{
		updateDatasetFunc: func(ctx context.Context, id string, req knowledge.DatasetMutationRequest) (*knowledge.Dataset, error) {
			captured = req
			dataset := knowledge.Dataset(req)
			return &dataset, nil
		},
	}
	svc := NewKnowledgeService(engine, setupKnowledgeProviderSvc(t))

	_, err := svc.UpdateDataset(context.Background(), "default", "kb1", knowledge.DatasetMutationRequest{
		"parser_config": map[string]any{
			"layout_recognize": "DeepDOC",
		},
	})
	require.NoError(t, err)

	parserConfig, ok := captured["parser_config"].(map[string]any)
	require.True(t, ok, "parser_config must remain a map[string]any after translation; got %T", captured["parser_config"])
	require.Equal(t, "DeepDOC", parserConfig["layout_recognize"])
}
