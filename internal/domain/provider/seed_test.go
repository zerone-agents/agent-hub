package provider

import "testing"

func TestSeeds_AllModelsHaveModelType(t *testing.T) {
	for _, spec := range BuiltinSeedSpecs() {
		for i, m := range spec.DefaultModels {
			if m.ModelType == "" {
				t.Errorf("%s model[%d] (%s) has empty ModelType", spec.Key, i, m.ModelID)
			}
		}
	}
}

func TestSeeds_LLMSeedsHaveLLMorVLMModelType(t *testing.T) {
	llmKeys := map[string]bool{
		"glm-cn": true, "kimi-cn": true, "bailian": true,
		"anthropic-thirdparty": true, "openai-thirdparty": true,
	}
	for _, spec := range BuiltinSeedSpecs() {
		if !llmKeys[spec.Key] {
			continue
		}
		for i, m := range spec.DefaultModels {
			if m.ModelType != string(TypeLLM) && m.ModelType != string(TypeVLM) {
				t.Errorf("%s model[%d] (%s): expected ModelType=llm or vlm, got %s", spec.Key, i, m.ModelID, m.ModelType)
			}
		}
	}
}

func TestSeeds_OCRSeedsHaveOCRModelType(t *testing.T) {
	ocrKeys := map[string]bool{"mineru": true, "paddleocr": true}
	for _, spec := range BuiltinSeedSpecs() {
		if !ocrKeys[spec.Key] {
			continue
		}
		for i, m := range spec.DefaultModels {
			if m.ModelType != string(TypeOCR) {
				t.Errorf("%s model[%d] (%s): expected ModelType=ocr, got %s", spec.Key, i, m.ModelID, m.ModelType)
			}
		}
	}
}

// TestSeeds_NewFromSeedSpecPreservesAllFields verifies that NewFromSeedSpec
// copies every spec field onto the constructed provider.
func TestSeeds_NewFromSeedSpecPreservesAllFields(t *testing.T) {
	spec := SeedSpec{
		Key:           "test-key",
		Name:          "Test",
		Description:   "desc-zh",
		DescriptionEn: "desc-en",
		Protocol:      string(ProtocolAnthropic),
		AuthStyle:     string(AuthStyleAPIKey),
		BaseURL:       "https://example.com",
		IconKey:       "test",
		Builtin:       true,
		DefaultModels: []CatalogModel{
			{ModelID: "m1", DisplayName: "M1", ModelType: string(TypeLLM)},
		},
		Fields: []PresetField{
			{Key: "api_key", Label: "Key", LabelEn: "Key", Type: "password"},
		},
	}

	p := NewFromSeedSpec(spec)
	if p.Key() != spec.Key {
		t.Errorf("Key = %q, want %q", p.Key(), spec.Key)
	}
	if p.Name() != spec.Name {
		t.Errorf("Name = %q, want %q", p.Name(), spec.Name)
	}
	if p.Description() != spec.Description {
		t.Errorf("Description = %q, want %q", p.Description(), spec.Description)
	}
	if p.DescriptionEn() != spec.DescriptionEn {
		t.Errorf("DescriptionEn = %q, want %q", p.DescriptionEn(), spec.DescriptionEn)
	}
	if p.Protocol() != spec.Protocol {
		t.Errorf("Protocol = %q, want %q", p.Protocol(), spec.Protocol)
	}
	if p.AuthStyle() != spec.AuthStyle {
		t.Errorf("AuthStyle = %q, want %q", p.AuthStyle(), spec.AuthStyle)
	}
	if p.BaseURL() != spec.BaseURL {
		t.Errorf("BaseURL = %q, want %q", p.BaseURL(), spec.BaseURL)
	}
	if p.IconKey() != spec.IconKey {
		t.Errorf("IconKey = %q, want %q", p.IconKey(), spec.IconKey)
	}
	if !p.Builtin() {
		t.Errorf("Builtin = false, want true")
	}
	if len(p.DefaultModels()) != 1 || p.DefaultModels()[0].ModelID != "m1" {
		t.Errorf("DefaultModels mismatch: %+v", p.DefaultModels())
	}
	if len(p.Fields()) != 1 || p.Fields()[0].Key != "api_key" {
		t.Errorf("Fields mismatch: %+v", p.Fields())
	}
}

// TestSeeds_BuiltinSeedSpecsHasAllExpectedKeys verifies the canonical seed
// list hasn't accidentally lost an entry during refactor.
func TestSeeds_BuiltinSeedSpecsHasAllExpectedKeys(t *testing.T) {
	want := map[string]bool{
		"glm-cn":               true,
		"kimi-cn":              true,
		"bailian":              true,
		"anthropic-thirdparty": true,
		"openai-thirdparty":    true,
		"mineru":               true,
		"paddleocr":            true,
	}
	got := map[string]bool{}
	for _, spec := range BuiltinSeedSpecs() {
		got[spec.Key] = true
	}
	for k := range want {
		if !got[k] {
			t.Errorf("BuiltinSeedSpecs() missing key %q", k)
		}
	}
	if len(got) != len(want) {
		t.Errorf("BuiltinSeedSpecs() returned %d entries, want %d", len(got), len(want))
	}
}
