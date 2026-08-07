package provider

import "fmt"

// AttrRule describes a single provider-specific config attribute for a
// given protocol. It is the data-driven source of truth consumed by both
// the backend validator (ValidateAttributes) and the frontend dynamic form
// renderer (served via the /admin/providers/attr-rules endpoint).
type AttrRule struct {
	Key      string   `json:"key"`
	Type     string   `json:"type"`     // "string" | "bool" | "int"
	Required bool     `json:"required"` // must be present & non-empty
	Enum     []string `json:"enum"`     // optional allow-list; nil = free value
	Default  string   `json:"default"`  // default value (as string)
	Label    string   `json:"label"`    // form label (zh)
	LabelEn  string   `json:"label_en"` // form label (en) / placeholder
}

// ProviderAttrRules maps a protocol to the attributes it expects.
// Adding a new provider-specific attribute is a one-line append here —
// no struct/column/DTO/form/handler changes required.
var ProviderAttrRules = map[string][]AttrRule{
	"mineru": {
		{
			Key:      "backend",
			Type:     "string",
			Required: true,
			Label:    "推理后端",
			LabelEn:  "Inference Backend",
			Enum: []string{
				"pipeline",
				"vlm-transformers",
				"vlm-vllm-engine",
				"vlm-http-client",
				"vlm-mlx-engine",
				"vlm-vllm-async-engine",
				"vlm-lmdeploy-engine",
			},
		},
		{Key: "delete_output", Type: "bool", Required: false, Default: "false", Label: "删除输出", LabelEn: "Delete Output"},
		{Key: "output_dir", Type: "string", Required: false, Default: "", Label: "输出目录", LabelEn: "Output Dir"},
	},
	"paddleocr": {
		{
			Key:      "algorithm",
			Type:     "string",
			Required: true,
			Default:  "PaddleOCR-VL",
			Label:    "OCR 算法",
			LabelEn:  "OCR Algorithm",
			Enum:     []string{"PaddleOCR-VL"},
		},
	},
	// Other protocols currently have no provider-specific attributes.
}

// Type constants for attributes.
const (
	AttrTypeString = "string"
	AttrTypeBool   = "bool"
	AttrTypeInt    = "int"
)

// ValidateAttributes checks the provided attribute map against the rules for
// the given protocol. Non-mineru protocols have empty rules and pass.
func ValidateAttributes(protocol string, attrs map[string]AttrValue) error {
	for _, rule := range ProviderAttrRules[protocol] {
		v, ok := attrs[rule.Key]

		if rule.Required && (!ok || v.Value == "") {
			return fmt.Errorf("protocol %s 要求必填属性: %s", protocol, rule.Key)
		}
		if !ok {
			continue
		}
		// Type check
		switch rule.Type {
		case AttrTypeBool:
			if v.Value != "true" && v.Value != "false" {
				return fmt.Errorf("属性 %s 必须是 bool（true/false），收到: %s", rule.Key, v.Value)
			}
		case AttrTypeInt:
			if !isInt(v.Value) {
				return fmt.Errorf("属性 %s 必须是整数，收到: %s", rule.Key, v.Value)
			}
		case AttrTypeString:
			// any string is acceptable
		default:
			return fmt.Errorf("属性 %s 类型不支持: %s", rule.Key, rule.Type)
		}
		// Enum allow-list
		if len(rule.Enum) > 0 && !contains(rule.Enum, v.Value) {
			return fmt.Errorf("属性 %s 取值不支持: %s（可选: %v）", rule.Key, v.Value, rule.Enum)
		}
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

func isInt(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
