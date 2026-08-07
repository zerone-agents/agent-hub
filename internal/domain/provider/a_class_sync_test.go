package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// mockMultiRAGClient records the last request it received.
type mockMultiRAGClient struct {
	lastAddLLM *AddLLMRequest
	addLLMResp *MultiRAGResponse
	addLLMErr  error
	calls      int
}

func (m *mockMultiRAGClient) AddLLM(ctx context.Context, payload AddLLMRequest) (*MultiRAGResponse, error) {
	m.calls++
	cp := payload
	m.lastAddLLM = &cp
	if m.addLLMErr != nil {
		return nil, m.addLLMErr
	}
	if m.addLLMResp != nil {
		return m.addLLMResp, nil
	}
	return &MultiRAGResponse{HTTPStatusCode: 200, Success: true, Message: "ok"}, nil
}

// TestSync_AnthropicCompatibleUsesAnthropicFactoryAddLLM verifies the named
// "anthropic-thirdparty" preset syncs under the Anthropic factory using
// per-model add_llm.
func TestSync_AnthropicCompatibleUsesAnthropicFactoryAddLLM(t *testing.T) {
	client := &mockMultiRAGClient{}
	p := NewAnthropicCompatible()
	p.Base().baseURL = "https://api.anthropic.com/v1"
	p.Base().lockedAPIKey = "anthropic-key"
	p.Base().defaultModels = []CatalogModel{
		{ModelID: "claude-sonnet-4", DisplayName: "Claude Sonnet 4", ContextWindow: 200000, ModelType: string(TypeLLM)},
		{ModelID: "claude-vision", DisplayName: "Claude Vision", ContextWindow: 8192, ModelType: string(TypeVLM)},
	}

	res, err := p.SyncToMultiRAG(context.Background(), client, SyncOptions{VerifyOnly: true})
	require.NoError(t, err)
	require.True(t, res.Success)
	require.Equal(t, "Anthropic", res.FactoryName)
	require.Equal(t, "add_llm", res.Endpoint)
	require.Equal(t, 2, res.CallCount)
	require.Equal(t, 2, client.calls)

	// lastAddLLM holds the second call.
	require.NotNil(t, client.lastAddLLM)
	require.Equal(t, "Anthropic", client.lastAddLLM.LLMFactory)
	require.Equal(t, "claude-vision", client.lastAddLLM.LLMName)
	require.Equal(t, "image2text", client.lastAddLLM.MdlType)
	require.Equal(t, "https://api.anthropic.com/v1", client.lastAddLLM.APIBase)
	require.Equal(t, 8192, client.lastAddLLM.MaxTokens)
	require.Equal(t, "\"anthropic-key\"", string(client.lastAddLLM.APIKey))
	require.True(t, client.lastAddLLM.Verify)
}

// TestSync_GenericAnthropicProviderPreservesBrandedBaseURL is the
// regression guard for brand-only LLM presets (glm-cn, kimi-cn, bailian)
// whose Go types were removed. After the consolidation these are
// reconstructed as GenericAnthropicProvider, which must still sync to
// MultiRAG under the Anthropic factory name with the brand's custom
// base_url preserved, now via per-model add_llm.
func TestSync_GenericAnthropicProviderPreservesBrandedBaseURL(t *testing.T) {
	client := &mockMultiRAGClient{}
	p := NewFromSeedSpec(SeedSpec{
		Key:       "glm-cn",
		Name:      "GLM Coding Plan",
		Protocol:  string(ProtocolAnthropic),
		AuthStyle: string(AuthStyleAPIKey),
		BaseURL:   "https://open.bigmodel.cn/api/anthropic",
	})
	p.Base().lockedAPIKey = "glm-key"
	p.Base().defaultModels = []CatalogModel{
		{ModelID: "GLM-5-Turbo", DisplayName: "GLM-5-Turbo", ContextWindow: 200000, ModelType: string(TypeLLM)},
	}

	res, err := p.SyncToMultiRAG(context.Background(), client, SyncOptions{})
	require.NoError(t, err)
	require.True(t, res.Success)
	require.Equal(t, "Anthropic", res.FactoryName)
	require.Equal(t, "add_llm", res.Endpoint)
	require.Equal(t, "https://open.bigmodel.cn/api/anthropic", client.lastAddLLM.APIBase)
	require.Equal(t, "\"glm-key\"", string(client.lastAddLLM.APIKey))
	require.Equal(t, "GLM-5-Turbo", client.lastAddLLM.LLMName)
	require.Equal(t, "chat", client.lastAddLLM.MdlType)
}

func TestSync_AnthropicPropagatesClientError(t *testing.T) {
	client := &mockMultiRAGClient{addLLMErr: errors.New("network down")}
	p := NewAnthropicCompatible()
	p.Base().baseURL = "https://x"
	p.Base().lockedAPIKey = "k"
	p.Base().defaultModels = []CatalogModel{
		{ModelID: "m1", ModelType: string(TypeLLM), ContextWindow: 8192},
	}

	res, err := p.SyncToMultiRAG(context.Background(), client, SyncOptions{})
	require.Error(t, err)
	require.Nil(t, res)
}

func TestSync_AnthropicReportsMultiRAGFailure(t *testing.T) {
	client := &mockMultiRAGClient{addLLMResp: &MultiRAGResponse{
		HTTPStatusCode: 400, Success: false, Message: "invalid api_key",
	}}
	p := NewAnthropicCompatible()
	p.Base().baseURL = "https://x"
	p.Base().lockedAPIKey = "k"
	p.Base().defaultModels = []CatalogModel{
		{ModelID: "m1", ModelType: string(TypeLLM), ContextWindow: 8192},
	}

	res, err := p.SyncToMultiRAG(context.Background(), client, SyncOptions{})
	require.NoError(t, err) // no transport error
	require.False(t, res.Success)
	require.Len(t, res.PerCall, 1)
	require.False(t, res.PerCall[0].OK)
	require.Contains(t, res.PerCall[0].Error, "invalid api_key")
}
