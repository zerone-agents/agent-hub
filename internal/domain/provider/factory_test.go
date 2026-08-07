package provider

import (
	"encoding/json"
	"testing"
)

func TestNewProvider(t *testing.T) {
	tests := []struct {
		key     string
		wantErr bool
	}{
		// Brand-only LLM keys (glm-cn, kimi-cn, bailian) were removed from
		// the factory: they are seeded via BuiltinSeedSpecs and reconstructed
		// from DB as generic providers (see NewProviderFromSummary).
		{"anthropic-thirdparty", false},
		{"openai-thirdparty", false},
		{"mineru", false},
		{"paddleocr", false},
		{"glm-cn", true},  // now unknown at factory level
		{"kimi-cn", true}, // now unknown at factory level
		{"bailian", true}, // now unknown at factory level
		{"unknown", true},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			p, err := NewProvider(tc.key)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for key %s", tc.key)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p == nil {
				t.Fatal("expected non-nil provider")
			}
			if p.Key() != tc.key {
				t.Errorf("expected key %s, got %s", tc.key, p.Key())
			}
		})
	}
}

func TestNewProviderFromSummary(t *testing.T) {
	fields := []PresetField{
		{Key: "api_key", Label: "API Key", LabelEn: "API Key", Type: "password", Secret: true},
	}
	fieldsJSON, _ := json.Marshal(fields)

	summary := &ProviderSummary{
		ID:           42,
		Key:          "custom-provider",
		Name:         "Custom Provider",
		Protocol:     string(ProtocolOpenAI),
		AuthStyle:    string(AuthStyleAPIKey),
		BaseURL:      "https://example.com",
		LockedAPIKey: "secret",
		Fields:       string(fieldsJSON),
		Builtin:      false,
	}

	p, err := NewProviderFromSummary(summary)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	if p.Key() != summary.Key {
		t.Errorf("expected key %s, got %s", summary.Key, p.Key())
	}
	if p.Name() != summary.Name {
		t.Errorf("expected name %s, got %s", summary.Name, p.Name())
	}
	if p.Protocol() != summary.Protocol {
		t.Errorf("expected protocol %s, got %s", summary.Protocol, p.Protocol())
	}
	// defaultModels are no longer loaded from summary.DefaultModels JSON;
	// the service must call SetDefaultModels after loading provider_models.
	if len(p.DefaultModels()) != 0 {
		t.Errorf("expected 0 models from summary, got %d", len(p.DefaultModels()))
	}
	if len(p.Fields()) != 1 {
		t.Errorf("expected 1 field, got %d", len(p.Fields()))
	}
	if p.LockedAPIKey() != "secret" {
		t.Errorf("expected locked api key secret, got %s", p.LockedAPIKey())
	}
}

func TestNewProviderFromSummary_KnownKey(t *testing.T) {
	summary := &ProviderSummary{
		Key:       "glm-cn",
		Name:      "Override Name",
		Protocol:  string(ProtocolAnthropic),
		AuthStyle: string(AuthStyleAPIKey),
		BaseURL:   "https://override.example.com",
	}

	p, err := NewProviderFromSummary(summary)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Key() != "glm-cn" {
		t.Errorf("expected key glm-cn, got %s", p.Key())
	}
	if p.Name() != "Override Name" {
		t.Errorf("expected name Override Name, got %s", p.Name())
	}
	if p.BaseURL() != "https://override.example.com" {
		t.Errorf("expected base url override, got %s", p.BaseURL())
	}
}

func TestNewProviderFromSummary_Nil(t *testing.T) {
	_, err := NewProviderFromSummary(nil)
	if err == nil {
		t.Fatal("expected error for nil summary")
	}
}
