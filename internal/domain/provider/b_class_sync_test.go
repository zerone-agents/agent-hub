package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSync_OpenAICompatible_LoopsOverModels(t *testing.T) {
	client := &mockMultiRAGClient{}
	p := NewOpenAICompatible()
	p.Base().baseURL = "https://api.openai.com/v1"
	p.Base().lockedAPIKey = "sk-test"
	// Replace defaultModels with a controlled set.
	p.Base().defaultModels = []CatalogModel{
		{ModelID: "gpt-4o", DisplayName: "GPT-4o", ContextWindow: 128000, ModelType: "llm"},
		{ModelID: "text-embedding-3", DisplayName: "Emb", ContextWindow: 8192, ModelType: "embedding"},
	}

	res, err := p.SyncToMultiRAG(context.Background(), client, SyncOptions{VerifyOnly: true})
	require.NoError(t, err)
	require.True(t, res.Success)
	require.Equal(t, "OpenAI-API-Compatible", res.FactoryName)
	require.Equal(t, "add_llm", res.Endpoint)
	require.Equal(t, 2, res.CallCount)
	require.Equal(t, 2, client.calls)

	// First call: gpt-4o, llm → chat, max_tokens = context_window.
	// Note: lastAddLLM only holds the LAST call, so we can only verify
	// the second call (text-embedding-3) directly here.
	require.Equal(t, "text-embedding-3", client.lastAddLLM.LLMName)
	require.Equal(t, "embedding", client.lastAddLLM.MdlType)
	require.Equal(t, 8192, client.lastAddLLM.MaxTokens)
	require.True(t, client.lastAddLLM.Verify)
}

func TestSync_OpenAICompatible_EmptyModelsReturnsEmptySuccess(t *testing.T) {
	client := &mockMultiRAGClient{}
	p := NewOpenAICompatible()
	p.Base().baseURL = "https://api.openai.com/v1"
	p.Base().lockedAPIKey = "sk-test"
	p.Base().defaultModels = nil

	res, err := p.SyncToMultiRAG(context.Background(), client, SyncOptions{})
	require.NoError(t, err)
	require.True(t, res.Success)
	require.Equal(t, 0, res.CallCount)
	require.Equal(t, 0, client.calls)
}

func TestSync_OpenAICompatible_PartialFailureAggregates(t *testing.T) {
	// Mock that fails every call.
	client := &mockMultiRAGClient{
		addLLMResp: &MultiRAGResponse{HTTPStatusCode: 400, Success: false, Message: "model not allowed"},
	}
	p := NewOpenAICompatible()
	p.Base().baseURL = "https://x"
	p.Base().lockedAPIKey = "k"
	p.Base().defaultModels = []CatalogModel{
		{ModelID: "m1", ModelType: "llm", ContextWindow: 8192},
		{ModelID: "m2", ModelType: "llm", ContextWindow: 8192},
	}

	res, err := p.SyncToMultiRAG(context.Background(), client, SyncOptions{})
	require.NoError(t, err)
	require.False(t, res.Success)
	require.Equal(t, 2, res.CallCount)
	require.Len(t, res.PerCall, 2)
	for _, c := range res.PerCall {
		require.False(t, c.OK)
		require.Contains(t, c.Error, "model not allowed")
	}
}

func TestSync_OpenAICompatible_SkipsUnknownModelType(t *testing.T) {
	client := &mockMultiRAGClient{}
	p := NewOpenAICompatible()
	p.Base().baseURL = "https://x"
	p.Base().lockedAPIKey = "k"
	p.Base().defaultModels = []CatalogModel{
		{ModelID: "good", ModelType: "llm", ContextWindow: 8192},
		{ModelID: "bad", ModelType: "unknown-type", ContextWindow: 8192},
	}

	res, err := p.SyncToMultiRAG(context.Background(), client, SyncOptions{})
	require.NoError(t, err)
	require.False(t, res.Success)
	require.Equal(t, 1, client.calls, "only the valid model should have been sent")
	require.Len(t, res.PerCall, 2)
	// First call: OK. Second: skipped with error explaining unmapped type.
	require.True(t, res.PerCall[0].OK)
	require.False(t, res.PerCall[1].OK)
	require.Contains(t, res.PerCall[1].Error, "unknown-type")
}

func TestSync_OpenAICompatible_NetworkErrorAborts(t *testing.T) {
	client := &mockMultiRAGClient{addLLMErr: errors.New("connection refused")}
	p := NewOpenAICompatible()
	p.Base().baseURL = "https://x"
	p.Base().lockedAPIKey = "k"
	p.Base().defaultModels = []CatalogModel{
		{ModelID: "m1", ModelType: "llm", ContextWindow: 8192},
	}

	res, err := p.SyncToMultiRAG(context.Background(), client, SyncOptions{})
	require.Error(t, err)
	require.Nil(t, res)
}
