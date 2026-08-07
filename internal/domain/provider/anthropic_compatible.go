package provider

import "context"

func NewAnthropicCompatible() *AnthropicCompatible {
	return &AnthropicCompatible{
		BaseProvider: BaseProvider{
			key:           "anthropic-thirdparty",
			name:          "Anthropic Compatible API",
			description:   "自定义 Anthropic 兼容 API，自行填写地址和密钥",
			descriptionEn: "Custom Anthropic-compatible API, fill in your own URL and key",
			protocol:      string(ProtocolAnthropic),
			authStyle:     string(AuthStyleAPIKey),
			baseURL:       "",
			defaultModels: []CatalogModel{},
			fields: []PresetField{
				{Key: "name", Label: "名称", LabelEn: "Name", Type: "text", Required: true},
				{Key: "base_url", Label: "API 地址", LabelEn: "API URL", Type: "text", Required: true},
				{Key: "api_key", Label: "API 密钥", LabelEn: "API Key", Type: "password", Secret: true},
			},
			iconKey: "anthropic",
			builtin: true,
		},
	}
}

type AnthropicCompatible struct{ BaseProvider }

func (p *AnthropicCompatible) SyncToMultiRAG(ctx context.Context, client MultiRAGClient, opts SyncOptions) (*SyncResult, error) {
	return p.syncAsAddLLM(ctx, client, opts, "Anthropic")
}

func (p *AnthropicCompatible) MultiRAGFactoryName() string { return "Anthropic" }
