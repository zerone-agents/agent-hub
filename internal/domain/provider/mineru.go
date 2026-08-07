package provider

import "context"

func NewMinerU() *MinerU {
	return &MinerU{
		BaseProvider: BaseProvider{
			key:           "mineru",
			name:          "MinerU",
			description:   "MinerU OCR 文档解析",
			descriptionEn: "MinerU OCR Document Parsing",
			protocol:      string(ProtocolMinerU),
			authStyle:     string(AuthStyleAPIKey),
			baseURL:       "",
			defaultModels: []CatalogModel{
				{ModelID: "mineru", DisplayName: "MinerU", ModelType: string(TypeOCR)},
			},
			fields: []PresetField{
				{Key: "api_key", Label: "API 密钥", LabelEn: "API Key", Type: "password", Secret: true},
			},
			iconKey: "mineru",
			builtin: true,
		},
	}
}

type MinerU struct{ BaseProvider }

func (p *MinerU) MultiRAGFactoryName() string { return "MinerU" }

func (p *MinerU) SyncToMultiRAG(ctx context.Context, client MultiRAGClient, opts SyncOptions) (*SyncResult, error) {
	nestedKey := map[string]any{
		"mineru_apiserver": p.Base().baseURL,
	}
	if v, ok := p.Base().attributes["backend"]; ok {
		nestedKey["mineru_backend"] = v.Value
	}
	if v, ok := p.Base().attributes["output_dir"]; ok {
		nestedKey["mineru_output_dir"] = v.Value
	}
	if v, ok := p.Base().attributes["delete_output"]; ok {
		if v.Value == "true" {
			nestedKey["mineru_delete_output"] = "1"
		} else {
			nestedKey["mineru_delete_output"] = "0"
		}
	}
	if v, ok := p.Base().attributes["server_url"]; ok {
		nestedKey["mineru_server_url"] = v.Value
	}
	return p.Base().syncAsAddLLMWithNestedKey(ctx, client, opts, "MinerU", nestedKey, nil)
}
