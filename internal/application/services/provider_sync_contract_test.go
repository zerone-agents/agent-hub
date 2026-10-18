package services

import (
	"context"
	"control-panel/internal/domain/provider"
	"control-panel/pkg/database"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

type cSyncCapture struct{ calls []provider.AddLLMRequest }

func (s *cSyncCapture) AddLLM(_ context.Context, payload provider.AddLLMRequest) (*provider.MultiRAGResponse, error) {
	s.calls = append(s.calls, payload)
	return &provider.MultiRAGResponse{Success: true, HTTPStatusCode: 200}, nil
}
func TestProviderSync_SelectedModelFailsBeforeUpstream(t *testing.T) {
	for _, mode := range []string{"missing", "disabled", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			svc := setupProviderService(t, "fixture")
			client := &cSyncCapture{}
			if mode != "missing" {
				row := provider.ProviderModel{ProviderID: 1, SelectionID: "one", ModelID: "embed", ModelType: "embedding", Status: "1"}
				if mode == "disabled" {
					row.Status = "0"
				}
				require.NoError(t, database.DB.Create(&row).Error)
			}
			if mode == "duplicate" {
				require.NoError(t, database.DB.Create(&provider.ProviderModel{ProviderID: 1, SelectionID: "two", ModelID: "embed", ModelType: "llm", Status: "1"}).Error)
			}
			_, err := svc.SyncProviderToMultiRAG(context.Background(), "", 1, client, false, []string{"embed"})
			require.Error(t, err)
			require.Empty(t, client.calls)
		})
	}
}
func TestProviderSync_PreservesUneditedToolCapabilityContract(t *testing.T) {
	svc := setupProviderService(t, "replacement-fixture")
	client := &cSyncCapture{}
	require.NoError(t, database.DB.Create(&provider.ProviderModel{ProviderID: 1, SelectionID: "embed", ModelID: "embed", ModelType: "embedding", Status: "1"}).Error)
	result, err := svc.SyncProviderToMultiRAG(context.Background(), "", 1, client, false, []string{"embed", "embed"})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Len(t, client.calls, 1)
	raw, err := json.Marshal(client.calls[0])
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body))
	require.NotContains(t, body, "is_tools")
	require.Equal(t, "replacement-fixture", body["api_key"])
	require.Equal(t, "embedding", body["mdl_type"])
}
