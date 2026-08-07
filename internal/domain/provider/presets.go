package provider

// SeedPreset is the "nice" representation used for initial data seeding.
// The service layer handles JSON marshalling and encryption before inserting.
type SeedPreset struct {
	Key           string
	Name          string
	Description   string
	DescriptionEn string
	Protocol      Protocol
	AuthStyle     AuthStyle
	BaseURL       string
	DefaultModels []CatalogModel
	Fields        []PresetField
	IconKey       string
	Builtin       bool
	LockedAPIKey  string
}

// SeedPresets returns the initial vendor presets to insert when the table is empty.
// Deprecated: use the typed provider constructors (NewGLM, NewKimi, etc.) instead.
// Kept for backward compatibility with callers that still expect the SeedPreset struct.
func SeedPresets() []SeedPreset {
	return []SeedPreset{
		{
			Key:           "anthropic-thirdparty",
			Name:          "Anthropic Compatible API",
			Description:   "自定义 Anthropic 兼容 API，自行填写地址和密钥",
			DescriptionEn: "Custom Anthropic-compatible API, fill in your own URL and key",
			Protocol:      ProtocolAnthropic,
			AuthStyle:     AuthStyleAPIKey,
			BaseURL:       "",
			DefaultModels: []CatalogModel{},
			Fields: []PresetField{
				{Key: "name", Label: "名称", LabelEn: "Name", Type: "text", Required: true},
				{Key: "base_url", Label: "API 地址", LabelEn: "API URL", Type: "text", Required: true},
				{Key: "api_key", Label: "API 密钥", LabelEn: "API Key", Type: "password", Secret: true},
			},
			IconKey: "anthropic",
		},
		{
			Key:           "openai-thirdparty",
			Name:          "OpenAI Compatible API",
			Description:   "自定义 OpenAI 兼容 API，自行填写地址和密钥",
			DescriptionEn: "Custom OpenAI-compatible API, fill in your own URL and key",
			Protocol:      ProtocolOpenAI,
			AuthStyle:     AuthStyleAuthToken,
			BaseURL:       "",
			DefaultModels: []CatalogModel{},
			Fields: []PresetField{
				{Key: "name", Label: "名称", LabelEn: "Name", Type: "text", Required: true},
				{Key: "base_url", Label: "API 地址", LabelEn: "API URL", Type: "text", Required: true},
				{Key: "api_key", Label: "API 密钥", LabelEn: "API Key", Type: "password", Secret: true},
			},
			IconKey: "openai",
		},
		{
			Key:           "glm-cn",
			Name:          "GLM Coding Plan",
			Description:   "智谱 GLM 编程套餐",
			DescriptionEn: "Zhipu GLM Coding Plan",
			Protocol:      ProtocolAnthropic,
			AuthStyle:     AuthStyleAPIKey,
			BaseURL:       "https://open.bigmodel.cn/api/anthropic",
			DefaultModels: []CatalogModel{
				{ModelID: "GLM-5-Turbo", DisplayName: "GLM-5-Turbo", ContextWindow: 200000, ModelType: string(TypeLLM)},
				{ModelID: "GLM-5.1", DisplayName: "GLM-5.1", ContextWindow: 200000, ModelType: string(TypeLLM)},
				{ModelID: "GLM-5.2", DisplayName: "GLM-5.2", ContextWindow: 1000000, ModelType: string(TypeLLM)},
			},
			Fields: []PresetField{
				{Key: "api_key", Label: "API 密钥", LabelEn: "API Key", Type: "password", Secret: true},
			},
			IconKey: "zhipu",
		},
		{
			Key:           "kimi-cn",
			Name:          "Kimi Code",
			Description:   "月之暗面 Kimi 编程套餐",
			DescriptionEn: "Moonshot Kimi For Coding",
			Protocol:      ProtocolAnthropic,
			AuthStyle:     AuthStyleAPIKey,
			BaseURL:       "https://api.kimi.com/coding",
			DefaultModels: []CatalogModel{
				{ModelID: "kimi-for-coding", DisplayName: "kimi-for-coding", ContextWindow: 256000, ModelType: string(TypeLLM)},
			},
			Fields: []PresetField{
				{Key: "api_key", Label: "API 密钥", LabelEn: "API Key", Type: "password", Secret: true},
			},
			IconKey: "kimi",
		},
		{
			Key:           "bailian",
			Name:          "Aliyun Bailian",
			Description:   "阿里云百炼 Coding Plan — 通义千问、GLM、Kimi、MiniMax",
			DescriptionEn: "Aliyun Bailian Coding Plan — Qwen, GLM, Kimi, MiniMax",
			Protocol:      ProtocolAnthropic,
			AuthStyle:     AuthStyleAPIKey,
			BaseURL:       "https://coding.dashscope.aliyuncs.com/apps/anthropic",
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
			IconKey: "bailian",
		},
	}
}
