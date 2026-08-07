package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSync_MinerU_BuildsNestedAPIKeyFromBaseURLAndAttributes(t *testing.T) {
	client := &mockMultiRAGClient{}
	p := NewMinerU()
	p.Base().baseURL = "http://mineru-host:9987"
	p.Base().lockedAPIKey = "ignored-for-mineru"
	p.Base().attributes = map[string]AttrValue{
		"backend":       {Type: "string", Value: "pipeline"},
		"output_dir":    {Type: "string", Value: "/tmp/mineru"},
		"delete_output": {Type: "bool", Value: "true"},
	}

	res, err := p.SyncToMultiRAG(context.Background(), client, SyncOptions{VerifyOnly: true})
	require.NoError(t, err)
	require.True(t, res.Success)
	require.Equal(t, "MinerU", res.FactoryName)
	require.Equal(t, "add_llm", res.Endpoint)

	require.NotNil(t, client.lastAddLLM)
	require.Equal(t, "MinerU", client.lastAddLLM.LLMFactory)
	require.Equal(t, "mineru", client.lastAddLLM.LLMName) // first model's ModelID
	require.Equal(t, "ocr", client.lastAddLLM.MdlType)
	require.True(t, client.lastAddLLM.Verify)

	var nested map[string]any
	require.NoError(t, json.Unmarshal(client.lastAddLLM.APIKey, &nested))
	require.Equal(t, "http://mineru-host:9987", nested["mineru_apiserver"])
	require.Equal(t, "pipeline", nested["mineru_backend"])
	require.Equal(t, "/tmp/mineru", nested["mineru_output_dir"])
	require.Equal(t, "1", nested["mineru_delete_output"])
	_, hasServerURL := nested["mineru_server_url"]
	require.False(t, hasServerURL)
}

func TestSync_MinerU_OmitsOptionalAttributesNotSet(t *testing.T) {
	client := &mockMultiRAGClient{}
	p := NewMinerU()
	p.Base().baseURL = "http://x"
	p.Base().attributes = map[string]AttrValue{
		"backend": {Type: "string", Value: "vlm-vllm-engine"},
	}

	_, err := p.SyncToMultiRAG(context.Background(), client, SyncOptions{})
	require.NoError(t, err)

	var nested map[string]any
	require.NoError(t, json.Unmarshal(client.lastAddLLM.APIKey, &nested))
	require.Contains(t, nested, "mineru_apiserver")
	require.Contains(t, nested, "mineru_backend")
	require.NotContains(t, nested, "mineru_output_dir")
	require.NotContains(t, nested, "mineru_delete_output")
}

func TestSync_PaddleOCR_BuildsNestedAPIKeyFromBaseURLAndAttributes(t *testing.T) {
	client := &mockMultiRAGClient{}
	p := NewPaddleOCR()
	p.Base().baseURL = "https://paddleocr-server/layout-parsing"
	p.Base().attributes = map[string]AttrValue{
		"algorithm": {Type: "string", Value: "PaddleOCR-VL"},
	}

	res, err := p.SyncToMultiRAG(context.Background(), client, SyncOptions{})
	require.NoError(t, err)
	require.True(t, res.Success)
	require.Equal(t, "PaddleOCR", res.FactoryName)

	require.NotNil(t, client.lastAddLLM)
	require.Equal(t, "PaddleOCR", client.lastAddLLM.LLMFactory)
	require.Equal(t, "paddleocr", client.lastAddLLM.LLMName)
	require.Equal(t, "ocr", client.lastAddLLM.MdlType)

	var nested map[string]any
	require.NoError(t, json.Unmarshal(client.lastAddLLM.APIKey, &nested))
	require.Equal(t, "https://paddleocr-server/layout-parsing", nested["paddleocr_api_url"])
	require.Equal(t, "PaddleOCR-VL", nested["paddleocr_algorithm"])
	_, hasToken := nested["paddleocr_access_token"]
	require.False(t, hasToken, "optional access_token should be omitted when not in attrs")
}

func TestSync_PaddleOCR_IncludesAccessTokenWhenPresent(t *testing.T) {
	client := &mockMultiRAGClient{}
	p := NewPaddleOCR()
	p.Base().baseURL = "https://x"
	p.Base().attributes = map[string]AttrValue{
		"algorithm":    {Type: "string", Value: "PaddleOCR-VL"},
		"access_token": {Type: "string", Value: "tok-123"},
	}

	_, err := p.SyncToMultiRAG(context.Background(), client, SyncOptions{})
	require.NoError(t, err)

	var nested map[string]any
	require.NoError(t, json.Unmarshal(client.lastAddLLM.APIKey, &nested))
	require.Equal(t, "tok-123", nested["paddleocr_access_token"])
}

func TestSync_MinerU_RequiresAtLeastOneModel(t *testing.T) {
	client := &mockMultiRAGClient{}
	p := NewMinerU()
	p.Base().baseURL = "http://x"
	p.Base().defaultModels = nil

	_, err := p.SyncToMultiRAG(context.Background(), client, SyncOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no models")
}
