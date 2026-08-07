package provider

import (
	"fmt"
)

// NewProvider instantiates a built-in provider by its key. Only providers
// that have protocol-specific behavior beyond BaseProvider get a case
// here; brand-only LLM presets (glm-cn, kimi-cn, bailian) are seeded via
// BuiltinSeedSpecs() and reconstructed from DB as generic providers.
func NewProvider(key string) (Provider, error) {
	switch key {
	case "anthropic-thirdparty":
		return NewAnthropicCompatible(), nil
	case "openai-thirdparty":
		return NewOpenAICompatible(), nil
	case "mineru":
		return NewMinerU(), nil
	case "paddleocr":
		return NewPaddleOCR(), nil
	default:
		return nil, fmt.Errorf("unknown provider key: %s", key)
	}
}

// NewProviderFromSummary constructs a Provider from a persisted summary.
// Models are NOT loaded here; the service layer loads them from the
// provider_models table and calls SetDefaultModels afterwards.
func NewProviderFromSummary(summary *ProviderSummary) (Provider, error) {
	if summary == nil {
		return nil, fmt.Errorf("provider summary is nil")
	}

	p, err := NewProvider(summary.Key)
	if err != nil {
		p = newGenericProvider(summary.Protocol)
	}

	base := p.Base()
	if base == nil {
		return nil, fmt.Errorf("provider %T does not expose a base provider", p)
	}

	if err := base.SetSummary(summary); err != nil {
		return nil, err
	}
	return p, nil
}
