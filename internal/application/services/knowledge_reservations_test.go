package services

import (
	"context"
	"testing"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/knowledge"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func reservedTestGuard(t *testing.T) (*DatasetGuard, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&knowledge.ReservedDataset{}, &agent.AgentKnowledgeDataset{}))
	g := NewDatasetGuard(db)
	require.NoError(t, g.Reserve(context.Background(), "memory-private", "example-tenant", "memory-app"))
	return g, db
}

func TestReservedMemoryDatasetCannotBypassThroughGeneralKnowledge(t *testing.T) {
	g, _ := reservedTestGuard(t)
	ctx := context.Background()
	calls := 0
	engine := &fakeKnowledgeEngine{getDatasetFunc: func(context.Context, string) (*knowledge.Dataset, error) { calls++; return nil, nil }, retrievalFunc: func(context.Context, knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error) {
		calls++
		return nil, nil
	}}
	svc := NewKnowledgeService(engine, nil)
	svc.SetDatasetGuard(g)
	_, err := svc.GetDataset(ctx, "MEMORY-PRIVATE")
	require.Equal(t, 404, knowledge.StatusCode(err))
	_, err = svc.ListDocuments(ctx, "memory-private", knowledge.DocumentListRequest{})
	require.Equal(t, 404, knowledge.StatusCode(err))
	_, err = svc.CreateChunk(ctx, "memory-private", "doc", knowledge.ChunkMutationRequest{"content": "x"})
	require.Equal(t, 404, knowledge.StatusCode(err))
	err = svc.DeleteDocuments(ctx, "memory-private", knowledge.DeleteRequest{DeleteAll: true})
	require.Equal(t, 404, knowledge.StatusCode(err))
	err = svc.DeleteDatasets(ctx, "default", knowledge.DeleteRequest{DeleteAll: true})
	require.Equal(t, 400, knowledge.StatusCode(err))
	for _, request := range []knowledge.RetrievalRequest{
		{"dataset_ids": []string{"memory-private"}, "question": "x"},
		{"dataset_ids": []string{"ordinary"}, "question": "x", "search_id": "saved-overrides-scope"},
		{"dataset_ids": []string{"ordinary"}, "question": "x", "search_id": 123},
	} {
		_, err = svc.Retrieval(ctx, request)
		require.Error(t, err)
	}
	_, err = svc.KnowledgePage(ctx, knowledge.PageRequest{Operation: "create-empty", DatasetID: "memory-private"})
	require.Error(t, err)
	_, err = svc.CreateDataset(ctx, "default", knowledge.DatasetMutationRequest{"name": "ordinary", "parser_config": map[string]any{"tag_kb_ids": []string{"memory-private"}}})
	require.Error(t, err)
	_, err = svc.UpdateDataset(ctx, "default", "ordinary", knowledge.DatasetMutationRequest{"parser_config": map[string]any{"tag_kb_ids": "memory-private"}})
	require.Error(t, err)
	err = svc.IngestDocuments(ctx, knowledge.DocumentIngestRequest{DocIDs: []string{"memory-document"}, Run: "1"})
	require.Error(t, err)
	require.Zero(t, calls, "blocked requests must never reach the backend")
	// The reservation remains active without an enabled memory service.
	svc2 := NewKnowledgeService(engine, nil)
	svc2.SetDatasetGuard(NewDatasetGuard(g.db))
	_, err = svc2.GetDataset(ctx, "memory-private")
	require.Error(t, err)
}

func TestReservedDatasetListAndOwnerProtection(t *testing.T) {
	g, db := reservedTestGuard(t)
	ctx := context.Background()
	engine := &fakeKnowledgeEngine{listDatasetsFunc: func(context.Context, knowledge.DatasetListRequest) (*knowledge.DatasetListResult, error) {
		return &knowledge.DatasetListResult{Total: 3, Datasets: []knowledge.Dataset{{"id": "public-a"}, {"id": "memory-private", "name": "private name"}, {"id": "public-b"}}}, nil
	}}
	svc := NewKnowledgeService(engine, nil)
	svc.SetDatasetGuard(g)
	out, err := svc.ListDatasets(ctx, knowledge.DatasetListRequest{Page: 2, PageSize: 1})
	require.NoError(t, err)
	require.Equal(t, 2, out.Total)
	require.Equal(t, "public-b", out.Datasets[0]["id"])
	require.Error(t, g.Reserve(ctx, "memory-private", "other-tenant", "other-app"))
	require.NoError(t, db.Create(&agent.AgentKnowledgeDataset{AgentID: 1, DatasetID: "bound"}).Error)
	require.Error(t, g.Reserve(ctx, "bound", "example-tenant", "memory-app"))
	// Agent binding validation happens before looking up/mutating an agent.
	a := &AgentService{datasetGuard: g}
	require.Error(t, a.UpdateAgentKnowledgeDatasets("default", "any", []string{"memory-private"}))
}
