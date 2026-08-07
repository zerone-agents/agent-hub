package provider

// SeedSpec is the data-only description of a built-in provider preset. It
// captures everything needed to seed a ProviderSummary row plus its child
// models, without requiring a dedicated Go type per brand.
//
// MultiRAG factory name is intentionally NOT part of this spec — after the
// consolidation, every LLM provider syncs to MultiRAG using the generic
// factory name of its protocol (Anthropic → "Anthropic", OpenAI →
// "OpenAI-API-Compatible"). OCR providers keep their own factory names
// via concrete type overrides (MinerU, PaddleOCR).
type SeedSpec struct {
	Key           string
	Name          string
	Description   string
	DescriptionEn string
	Protocol      string
	AuthStyle     string
	BaseURL       string
	IconKey       string
	Builtin       bool
	DefaultModels []CatalogModel
	Fields        []PresetField
}

// BuiltinSeedSpecs returns the full list of provider presets inserted by
// SeedIfEmpty on first startup. The list is data-only — no behavior. All
// LLM entries use anthropic or openai protocol and sync to MultiRAG under
// the protocol's default factory name.
func BuiltinSeedSpecs() []SeedSpec {
	return []SeedSpec{
		// ── LLM (Anthropic-compatible) ───────────────────────────────
		{
			Key:           "glm-cn",
			Name:          "GLM Coding Plan",
			Description:   "智谱 GLM 编程套餐",
			DescriptionEn: "Zhipu GLM Coding Plan",
			Protocol:      string(ProtocolAnthropic),
			AuthStyle:     string(AuthStyleAPIKey),
			BaseURL:       "https://open.bigmodel.cn/api/anthropic",
			IconKey:       "zhipu",
			Builtin:       true,
			DefaultModels: []CatalogModel{
				{ModelID: "GLM-5-Turbo", DisplayName: "GLM-5-Turbo", ContextWindow: 200000, ModelType: string(TypeLLM)},
				{ModelID: "GLM-5.1", DisplayName: "GLM-5.1", ContextWindow: 200000, ModelType: string(TypeLLM)},
				{ModelID: "GLM-5.2", DisplayName: "GLM-5.2", ContextWindow: 1000000, ModelType: string(TypeLLM)},
			},
			Fields: []PresetField{
				{Key: "api_key", Label: "API 密钥", LabelEn: "API Key", Type: "password", Secret: true},
			},
		},
		{
			Key:           "kimi-cn",
			Name:          "Kimi Code",
			Description:   "月之暗面 Kimi 编程套餐",
			DescriptionEn: "Moonshot Kimi For Coding",
			Protocol:      string(ProtocolAnthropic),
			AuthStyle:     string(AuthStyleAPIKey),
			BaseURL:       "https://api.kimi.com/coding",
			IconKey:       "kimi",
			Builtin:       true,
			DefaultModels: []CatalogModel{
				{ModelID: "kimi-for-coding", DisplayName: "kimi-for-coding", ContextWindow: 256000, ModelType: string(TypeLLM)},
			},
			Fields: []PresetField{
				{Key: "api_key", Label: "API 密钥", LabelEn: "API Key", Type: "password", Secret: true},
			},
		},
		{
			Key:           "bailian",
			Name:          "Aliyun Bailian",
			Description:   "阿里云百炼 Coding Plan — 通义千问、GLM、Kimi、MiniMax",
			DescriptionEn: "Aliyun Bailian Coding Plan — Qwen, GLM, Kimi, MiniMax",
			Protocol:      string(ProtocolAnthropic),
			AuthStyle:     string(AuthStyleAPIKey),
			BaseURL:       "https://coding.dashscope.aliyuncs.com/apps/anthropic",
			IconKey:       "bailian",
			Builtin:       true,
			DefaultModels: []CatalogModel{
				{ModelID: "qwen3.6-plus", DisplayName: "Qwen 3.6 Plus", ContextWindow: 1000000, ModelType: string(TypeLLM)},
				{ModelID: "qwen3.5-plus", DisplayName: "Qwen 3.5 Plus", ContextWindow: 1000000, ModelType: string(TypeLLM)},
				{ModelID: "qwen3-coder-next", DisplayName: "Qwen 3 Coder Next", ContextWindow: 200000, ModelType: string(TypeLLM)},
				{ModelID: "qwen3-coder-plus", DisplayName: "Qwen 3 Coder Plus", ContextWindow: 200000, ModelType: string(TypeLLM)},
				{ModelID: "kimi-k2.5", DisplayName: "Kimi K2.5", ContextWindow: 200000, ModelType: string(TypeVLM)},
				{ModelID: "glm-5", DisplayName: "GLM-5", ContextWindow: 200000, ModelType: string(TypeLLM)},
				{ModelID: "MiniMax-M2.5", DisplayName: "MiniMax-M2.5", ContextWindow: 200000, ModelType: string(TypeLLM)},
			},
			Fields: []PresetField{
				{Key: "api_key", Label: "API 密钥", LabelEn: "API Key", Type: "password", Secret: true},
			},
		},
		{
			Key:           "anthropic-thirdparty",
			Name:          "Anthropic Compatible API",
			Description:   "自定义 Anthropic 兼容 API，自行填写地址和密钥",
			DescriptionEn: "Custom Anthropic-compatible API, fill in your own URL and key",
			Protocol:      string(ProtocolAnthropic),
			AuthStyle:     string(AuthStyleAPIKey),
			BaseURL:       "",
			IconKey:       "anthropic",
			Builtin:       true,
			DefaultModels: []CatalogModel{},
			Fields: []PresetField{
				{Key: "name", Label: "名称", LabelEn: "Name", Type: "text", Required: true},
				{Key: "base_url", Label: "API 地址", LabelEn: "API URL", Type: "text", Required: true},
				{Key: "api_key", Label: "API 密钥", LabelEn: "API Key", Type: "password", Secret: true},
			},
		},
		// ── LLM (OpenAI-compatible) ─────────────────────────────────
		{
			Key:           "openai-thirdparty",
			Name:          "OpenAI Compatible API",
			Description:   "自定义 OpenAI 兼容 API，自行填写地址和密钥",
			DescriptionEn: "Custom OpenAI-compatible API, fill in your own URL and key",
			Protocol:      string(ProtocolOpenAI),
			AuthStyle:     string(AuthStyleAPIKey),
			BaseURL:       "",
			IconKey:       "openai",
			Builtin:       true,
			DefaultModels: []CatalogModel{},
			Fields: []PresetField{
				{Key: "name", Label: "名称", LabelEn: "Name", Type: "text", Required: true},
				{Key: "base_url", Label: "API 地址", LabelEn: "API URL", Type: "text", Required: true},
				{Key: "api_key", Label: "API 密钥", LabelEn: "API Key", Type: "password", Secret: true},
			},
		},
		// ── OCR ──────────────────────────────────────────────────────
		{
			Key:           "mineru",
			Name:          "MinerU",
			Description:   "MinerU OCR 文档解析",
			DescriptionEn: "MinerU OCR Document Parsing",
			Protocol:      string(ProtocolMinerU),
			AuthStyle:     string(AuthStyleAPIKey),
			BaseURL:       "",
			IconKey:       "mineru",
			Builtin:       true,
			DefaultModels: []CatalogModel{
				{ModelID: "mineru", DisplayName: "MinerU", ModelType: string(TypeOCR)},
			},
			Fields: []PresetField{
				{Key: "api_key", Label: "API 密钥", LabelEn: "API Key", Type: "password", Secret: true},
			},
		},
		{
			Key:           "paddleocr",
			Name:          "PaddleOCR",
			Description:   "PaddleOCR 文档解析",
			DescriptionEn: "PaddleOCR Document Parsing",
			Protocol:      string(ProtocolPaddleOCR),
			AuthStyle:     string(AuthStyleNoAuth),
			BaseURL:       "",
			IconKey:       "paddleocr",
			Builtin:       true,
			DefaultModels: []CatalogModel{
				{ModelID: "paddleocr", DisplayName: "PaddleOCR", ModelType: string(TypeOCR)},
			},
			Fields: []PresetField{},
		},
	}
}

// NewFromSeedSpec constructs a Provider from a seed spec by instantiating
// the generic provider for the spec's protocol and copying all spec
// fields into the BaseProvider. The returned provider has no
// protocol-specific behavior beyond what the generic type provides.
func NewFromSeedSpec(s SeedSpec) Provider {
	p := newGenericProvider(s.Protocol)
	base := p.Base()
	base.key = s.Key
	base.name = s.Name
	base.description = s.Description
	base.descriptionEn = s.DescriptionEn
	base.protocol = s.Protocol
	base.authStyle = s.AuthStyle
	base.baseURL = s.BaseURL
	base.iconKey = s.IconKey
	base.builtin = s.Builtin
	base.defaultModels = s.DefaultModels
	base.fields = s.Fields
	return p
}
