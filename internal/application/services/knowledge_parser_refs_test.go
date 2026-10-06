package services

import (
	"context"
	"testing"

	"control-panel/internal/domain/knowledge"
	"control-panel/internal/domain/provider"
	"control-panel/pkg/database"
	"github.com/stretchr/testify/require"
)

func TestKnowledgeService_ExplicitParserRefsAndEmbeddingStayIndependent(t *testing.T) {
	for _, layout := range []string{"vision@preview@OpenAI", "pipeline-one@MinerU", "pipeline-two@MinerU", "PP-OCRv5@PaddleOCR", "removed-vision@OpenAI"} {
		t.Run(layout, func(t *testing.T) {
			var captured knowledge.DatasetMutationRequest
			engine := &fakeKnowledgeEngine{updateDatasetFunc: func(ctx context.Context, id string, req knowledge.DatasetMutationRequest) (*knowledge.Dataset, error) {
				captured = req
				dataset := knowledge.Dataset(req)
				return &dataset, nil
			}}
			svc := NewKnowledgeService(engine, setupKnowledgeProviderSvc(t))
			_, err := svc.UpdateDataset(context.Background(), "default", "kb1", knowledge.DatasetMutationRequest{
				"embd_id":       "bge-large-zh@Anthropic",
				"parser_config": map[string]any{"layout_recognize": layout, "custom": "preserved"},
			})
			require.NoError(t, err)
			require.Equal(t, "bge-large-zh@Anthropic", captured["embd_id"])
			require.Equal(t, map[string]any{"layout_recognize": layout, "custom": "preserved"}, captured["parser_config"])
		})
	}
}

func TestKnowledgeService_LocalEmbeddingNameWithRevisionGetsItsFactory(t *testing.T) {
	providerSvc := setupKnowledgeProviderSvc(t)
	var existing provider.ProviderModel
	require.NoError(t, database.GetDB().Where("model_id = ?", "bge-large-zh").First(&existing).Error)
	require.NoError(t, database.GetDB().Create(&provider.ProviderModel{
		ProviderID: existing.ProviderID, SelectionID: "embed@revision", ModelID: "embed@revision",
		DisplayName: "Embedding revision", ModelType: string(provider.TypeEmbedding), Status: "1",
	}).Error)
	var captured knowledge.DatasetMutationRequest
	engine := &fakeKnowledgeEngine{updateDatasetFunc: func(ctx context.Context, id string, req knowledge.DatasetMutationRequest) (*knowledge.Dataset, error) {
		captured = req
		dataset := knowledge.Dataset(req)
		return &dataset, nil
	}}
	svc := NewKnowledgeService(engine, providerSvc)
	_, err := svc.UpdateDataset(context.Background(), "default", "kb1", knowledge.DatasetMutationRequest{
		"embd_id":       "embed@revision",
		"parser_config": map[string]any{"layout_recognize": "vision@revision@OpenAI"},
	})
	require.NoError(t, err)
	require.Equal(t, "embed@revision@Anthropic", captured["embd_id"])
	require.Equal(t, "vision@revision@OpenAI", captured["parser_config"].(map[string]any)["layout_recognize"])
}
