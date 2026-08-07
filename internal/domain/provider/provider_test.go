package provider

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBaseProvider_toSummary(t *testing.T) {
	baseTime := mustParseTime("2026-07-17T10:00:00Z")

	t.Run("normal case with fields", func(t *testing.T) {
		bp := &BaseProvider{
			id:            1,
			key:           "test-provider",
			name:          "Test Provider",
			description:   "A test provider",
			descriptionEn: "A test provider (en)",
			protocol:      "openai",
			authStyle:     "api_key",
			baseURL:       "https://api.test.example.com",
			lockedAPIKey:  "secret-key",
			iconKey:       "test-icon",
			builtin:       true,
			createdAt:     baseTime,
			updatedAt:     baseTime,
			defaultModels: []CatalogModel{
				{ModelID: "model-a", DisplayName: "Model A", ContextWindow: 4096},
				{ModelID: "model-b", DisplayName: "Model B", ContextWindow: 8192},
			},
			fields: []PresetField{
				{Key: "apiKey", Label: "API Key", LabelEn: "API Key", Type: "text", Required: true, Secret: true},
				{Key: "baseURL", Label: "Base URL", LabelEn: "Base URL", Type: "text"},
			},
		}

		summary := bp.ToSummary()
		if summary == nil {
			t.Fatal("expected non-nil summary")
		}

		assertSummaryMatches(t, summary, bp)

		var fields []PresetField
		if err := json.Unmarshal([]byte(summary.Fields), &fields); err != nil {
			t.Fatalf("failed to unmarshal Fields: %v", err)
		}
		if len(fields) != 2 {
			t.Fatalf("expected 2 fields, got %d", len(fields))
		}
		if fields[0].Key != "apiKey" {
			t.Errorf("expected field[0].Key to be apiKey, got %s", fields[0].Key)
		}
		if fields[1].Key != "baseURL" {
			t.Errorf("expected field[1].Key to be baseURL, got %s", fields[1].Key)
		}
	})

	t.Run("empty fields serialize to []", func(t *testing.T) {
		bp := &BaseProvider{
			id:            2,
			key:           "empty-provider",
			name:          "Empty Provider",
			defaultModels: []CatalogModel{},
			fields:        []PresetField{},
		}

		summary := bp.ToSummary()
		if summary == nil {
			t.Fatal("expected non-nil summary")
		}
		if summary.Fields != "[]" {
			t.Errorf("expected Fields to be [], got %s", summary.Fields)
		}
	})

	t.Run("nil fields serialize to []", func(t *testing.T) {
		bp := &BaseProvider{
			id:   3,
			key:  "nil-provider",
			name: "Nil Provider",
		}

		summary := bp.ToSummary()
		if summary == nil {
			t.Fatal("expected non-nil summary")
		}
		if summary.Fields != "[]" {
			t.Errorf("expected Fields to be [], got %s", summary.Fields)
		}
	})
}

func assertSummaryMatches(t *testing.T, summary *ProviderSummary, bp *BaseProvider) {
	t.Helper()

	if summary.ID != bp.id {
		t.Errorf("expected ID %d, got %d", bp.id, summary.ID)
	}
	if summary.Key != bp.key {
		t.Errorf("expected Key %s, got %s", bp.key, summary.Key)
	}
	if summary.Name != bp.name {
		t.Errorf("expected Name %s, got %s", bp.name, summary.Name)
	}
	if summary.Description != bp.description {
		t.Errorf("expected Description %s, got %s", bp.description, summary.Description)
	}
	if summary.DescriptionEn != bp.descriptionEn {
		t.Errorf("expected DescriptionEn %s, got %s", bp.descriptionEn, summary.DescriptionEn)
	}
	if summary.Protocol != bp.protocol {
		t.Errorf("expected Protocol %s, got %s", bp.protocol, summary.Protocol)
	}
	if summary.AuthStyle != bp.authStyle {
		t.Errorf("expected AuthStyle %s, got %s", bp.authStyle, summary.AuthStyle)
	}
	if summary.BaseURL != bp.baseURL {
		t.Errorf("expected BaseURL %s, got %s", bp.baseURL, summary.BaseURL)
	}
	if summary.LockedAPIKey != bp.lockedAPIKey {
		t.Errorf("expected LockedAPIKey %s, got %s", bp.lockedAPIKey, summary.LockedAPIKey)
	}
	if summary.IconKey != bp.iconKey {
		t.Errorf("expected IconKey %s, got %s", bp.iconKey, summary.IconKey)
	}
	if summary.Builtin != bp.builtin {
		t.Errorf("expected Builtin %v, got %v", bp.builtin, summary.Builtin)
	}
	if !summary.CreatedAt.Equal(bp.createdAt) {
		t.Errorf("expected CreatedAt %v, got %v", bp.createdAt, summary.CreatedAt)
	}
	if !summary.UpdatedAt.Equal(bp.updatedAt) {
		t.Errorf("expected UpdatedAt %v, got %v", bp.updatedAt, summary.UpdatedAt)
	}
}

func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
