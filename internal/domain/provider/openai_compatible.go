package provider

import (
	"context"
)

func NewOpenAICompatible() *OpenAICompatible {
	return &OpenAICompatible{
		BaseProvider: BaseProvider{
			key:           "openai-thirdparty",
			name:          "OpenAI Compatible API",
			description:   "自定义 OpenAI 兼容 API，自行填写地址和密钥",
			descriptionEn: "Custom OpenAI-compatible API, fill in your own URL and key",
			protocol:      string(ProtocolOpenAI),
			authStyle:     string(AuthStyleAPIKey),
			baseURL:       "",
			defaultModels: []CatalogModel{},
			fields: []PresetField{
				{Key: "name", Label: "名称", LabelEn: "Name", Type: "text", Required: true},
				{Key: "base_url", Label: "API 地址", LabelEn: "API URL", Type: "text", Required: true},
				{Key: "api_key", Label: "API 密钥", LabelEn: "API Key", Type: "password", Secret: true},
			},
			iconKey: "openai",
			builtin: true,
		},
	}
}

type OpenAICompatible struct{ BaseProvider }

func (p *OpenAICompatible) MultiRAGFactoryName() string { return "OpenAI-API-Compatible" }

func (p *OpenAICompatible) SyncToMultiRAG(ctx context.Context, client MultiRAGClient, opts SyncOptions) (*SyncResult, error) {
	return p.syncAsAddLLM(ctx, client, opts, "OpenAI-API-Compatible")
}
