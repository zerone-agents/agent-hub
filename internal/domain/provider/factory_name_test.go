package provider

import "testing"

func TestMultiRAGFactoryName(t *testing.T) {
	tests := []struct {
		name string
		p    Provider
		want string
	}{
		// GLM/Kimi/Bailian were removed; they used to return ZHIPU-AI /
		// Moonshot / Tongyi-Qianwen but now sync via the generic
		// "Anthropic" factory name (AnthropicCompatible).
		{"Anthropic", NewAnthropicCompatible(), "Anthropic"},
		{"OpenAI", NewOpenAICompatible(), "OpenAI-API-Compatible"},
		{"MinerU", NewMinerU(), "MinerU"},
		{"PaddleOCR", NewPaddleOCR(), "PaddleOCR"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.p.MultiRAGFactoryName(); got != tc.want {
				t.Errorf("%s.MultiRAGFactoryName() = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

// Default BaseProvider implementation returns empty string for any future
// provider that doesn't override.
func TestMultiRAGFactoryName_DefaultIsEmpty(t *testing.T) {
	b := &BaseProvider{}
	if got := b.MultiRAGFactoryName(); got != "" {
		t.Errorf("default factory name = %q, want empty", got)
	}
}

// Generic providers (used by NewFromSeedSpec for brand-only LLM presets)
// override MultiRAGFactoryName to return the protocol's default factory
// name, so that branded DB rows (glm-cn, kimi-cn, bailian) still sync
// to MultiRAG with a valid factory identifier after the type
// consolidation.
func TestMultiRAGFactoryName_GenericProvidersUseProtocolDefault(t *testing.T) {
	cases := []struct {
		protocol string
		want     string
	}{
		{string(ProtocolAnthropic), "Anthropic"},
		{string(ProtocolOpenAI), "OpenAI-API-Compatible"},
	}
	for _, tc := range cases {
		p := newGenericProvider(tc.protocol)
		if got := p.MultiRAGFactoryName(); got != tc.want {
			t.Errorf("generic %s provider factory name = %q, want %q", tc.protocol, got, tc.want)
		}
	}
}
