package provider

import "context"

// GenericAnthropicProvider and GenericOpenAIProvider are the protocol-level
// fallbacks used when reconstructing a Provider from a DB summary whose key
// is no longer in the factory (e.g. brand-only LLM presets like glm-cn,
// kimi-cn, bailian after the type consolidation). They implement
// SyncToMultiRAG so that branded DB rows still sync to MultiRAG under the
// protocol's default factory name using per-model add_llm.
type GenericAnthropicProvider struct{ BaseProvider }
type GenericOpenAIProvider struct{ BaseProvider }
type GenericMinerUProvider struct{ BaseProvider }
type GenericPaddleOCRProvider struct{ BaseProvider }

func (p *GenericAnthropicProvider) SyncToMultiRAG(ctx context.Context, client MultiRAGClient, opts SyncOptions) (*SyncResult, error) {
	return p.syncAsAddLLM(ctx, client, opts, "Anthropic")
}

func (p *GenericAnthropicProvider) MultiRAGFactoryName() string { return "Anthropic" }

func (p *GenericOpenAIProvider) SyncToMultiRAG(ctx context.Context, client MultiRAGClient, opts SyncOptions) (*SyncResult, error) {
	return p.syncAsAddLLM(ctx, client, opts, "OpenAI-API-Compatible")
}

func (p *GenericOpenAIProvider) MultiRAGFactoryName() string { return "OpenAI-API-Compatible" }

func newGenericProvider(protocol string) Provider {
	switch protocol {
	case string(ProtocolAnthropic):
		return &GenericAnthropicProvider{}
	case string(ProtocolOpenAI):
		return &GenericOpenAIProvider{}
	case string(ProtocolMinerU):
		return &GenericMinerUProvider{}
	case string(ProtocolPaddleOCR):
		return &GenericPaddleOCRProvider{}
	default:
		return &GenericOpenAIProvider{}
	}
}

// NewGenericProvider creates a generic provider for the given protocol.
func NewGenericProvider(protocol string) Provider {
	return newGenericProvider(protocol)
}
