package provider

import "context"

func NewPaddleOCR() *PaddleOCR {
	return &PaddleOCR{
		BaseProvider: BaseProvider{
			key:           "paddleocr",
			name:          "PaddleOCR",
			description:   "PaddleOCR 文档解析",
			descriptionEn: "PaddleOCR Document Parsing",
			protocol:      string(ProtocolPaddleOCR),
			authStyle:     string(AuthStyleNoAuth),
			baseURL:       "",
			defaultModels: []CatalogModel{
				{ModelID: "paddleocr", DisplayName: "PaddleOCR", ModelType: string(TypeOCR)},
			},
			fields:  []PresetField{},
			iconKey: "paddleocr",
			builtin: true,
		},
	}
}

type PaddleOCR struct{ BaseProvider }

func (p *PaddleOCR) MultiRAGFactoryName() string { return "PaddleOCR" }

func (p *PaddleOCR) SyncToMultiRAG(ctx context.Context, client MultiRAGClient, opts SyncOptions) (*SyncResult, error) {
	nestedKey := map[string]any{
		"paddleocr_api_url": p.Base().baseURL,
	}
	if v, ok := p.Base().attributes["algorithm"]; ok {
		nestedKey["paddleocr_algorithm"] = v.Value
	}
	if v, ok := p.Base().attributes["access_token"]; ok {
		nestedKey["paddleocr_access_token"] = v.Value
	}
	return p.Base().syncAsAddLLMWithNestedKey(ctx, client, opts, "PaddleOCR", nestedKey, nil)
}
