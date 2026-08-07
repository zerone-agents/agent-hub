package provider

import "testing"

func TestEnsureSelectionIDs_GeneratesFromModelID(t *testing.T) {
	models := []CatalogModel{
		{ModelID: "kimi-k3", DisplayName: "Kimi-K3"},
		{ModelID: "glm-5", DisplayName: "GLM-5"},
	}
	got := EnsureSelectionIDs(models)
	if got[0].SelectionID != "kimi-k3" {
		t.Errorf("got %q, want %q", got[0].SelectionID, "kimi-k3")
	}
	if got[1].SelectionID != "glm-5" {
		t.Errorf("got %q, want %q", got[1].SelectionID, "glm-5")
	}
}

func TestEnsureSelectionIDs_DeduplicatesWithinProvider(t *testing.T) {
	models := []CatalogModel{
		{ModelID: "kimi-k3", DisplayName: "Kimi-K3"},
		{ModelID: "kimi-k3", DisplayName: "Kimi-K3 (1M)"},
		{ModelID: "kimi-k3", DisplayName: "Kimi-K3 (Preview)"},
	}
	got := EnsureSelectionIDs(models)
	want := []string{"kimi-k3", "kimi-k3-2", "kimi-k3-3"}
	for i, w := range want {
		if got[i].SelectionID != w {
			t.Errorf("models[%d]: got %q, want %q", i, got[i].SelectionID, w)
		}
	}
}

func TestEnsureSelectionIDs_PreservesExisting(t *testing.T) {
	models := []CatalogModel{
		{SelectionID: "stable-id", ModelID: "kimi-k3", DisplayName: "Renamed"},
		{ModelID: "kimi-k3", DisplayName: "Kimi-K3"},
	}
	got := EnsureSelectionIDs(models)
	if got[0].SelectionID != "stable-id" {
		t.Errorf("existing id changed: got %q, want %q", got[0].SelectionID, "stable-id")
	}
	// 已有 "stable-id" 不占用 "kimi-k3"，新项仍可拿到 modelId 本身
	if got[1].SelectionID != "kimi-k3" {
		t.Errorf("got %q, want %q", got[1].SelectionID, "kimi-k3")
	}
}

func TestEnsureSelectionIDs_ExistingIDParticipatesInDedup(t *testing.T) {
	models := []CatalogModel{
		{SelectionID: "kimi-k3", ModelID: "other", DisplayName: "Other"},
		{ModelID: "kimi-k3", DisplayName: "Kimi-K3"},
	}
	got := EnsureSelectionIDs(models)
	if got[1].SelectionID != "kimi-k3-2" {
		t.Errorf("got %q, want %q", got[1].SelectionID, "kimi-k3-2")
	}
}

func TestEnsureSelectionIDs_EmptyModelID(t *testing.T) {
	models := []CatalogModel{
		{ModelID: "", DisplayName: "No ID"},
		{ModelID: "", DisplayName: "No ID 2"},
	}
	got := EnsureSelectionIDs(models)
	if got[0].SelectionID != "model" {
		t.Errorf("got %q, want %q", got[0].SelectionID, "model")
	}
	if got[1].SelectionID != "model-2" {
		t.Errorf("got %q, want %q", got[1].SelectionID, "model-2")
	}
}

func TestEnsureSelectionIDs_Idempotent(t *testing.T) {
	models := []CatalogModel{
		{ModelID: "kimi-k3", DisplayName: "Kimi-K3"},
		{ModelID: "kimi-k3", DisplayName: "Kimi-K3 (1M)"},
	}
	first := EnsureSelectionIDs(models)
	snapshot := make([]string, len(first))
	for i, m := range first {
		snapshot[i] = m.SelectionID
	}
	second := EnsureSelectionIDs(first)
	for i, m := range second {
		if m.SelectionID != snapshot[i] {
			t.Errorf("not idempotent: models[%d] changed from %q to %q", i, snapshot[i], m.SelectionID)
		}
	}
}

func TestHasMissingSelectionIDs(t *testing.T) {
	if HasMissingSelectionIDs([]CatalogModel{{SelectionID: "a"}}) {
		t.Error("want false when all have ids")
	}
	if !HasMissingSelectionIDs([]CatalogModel{{SelectionID: "a"}, {}}) {
		t.Error("want true when any id missing")
	}
	if HasMissingSelectionIDs(nil) {
		t.Error("want false for empty list")
	}
}
