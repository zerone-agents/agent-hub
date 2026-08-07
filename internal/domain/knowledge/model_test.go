package knowledge

import "testing"

func TestNormalizeDataset_UsesFutureDisplayNameAndCollectionName(t *testing.T) {
	dataset := NormalizeDataset(map[string]any{
		"id":              "kb1",
		"name":            "kb_physical_one",
		"display_name":    "校园网服务指南",
		"collection_name": "kb_physical_one",
	})
	got := map[string]any(dataset)
	if got["name"] != "校园网服务指南" {
		t.Fatalf("name = %q, want display name", got["name"])
	}
	if got["display_name"] != "校园网服务指南" {
		t.Fatalf("display_name = %q", got["display_name"])
	}
	if got["collection_name"] != "kb_physical_one" {
		t.Fatalf("collection_name = %q", got["collection_name"])
	}
}

func TestNormalizeDataset_UsesControlPanelDisplayNameBridge(t *testing.T) {
	dataset := NormalizeDataset(map[string]any{
		"id":   "kb1",
		"name": "kb_physical_one",
		"parser_config": map[string]any{
			"control_panel": map[string]any{
				"display_name": "校园网服务指南",
			},
		},
	})
	got := map[string]any(dataset)
	if got["name"] != "校园网服务指南" || got["display_name"] != "校园网服务指南" {
		t.Fatalf("display bridge mapping failed: %#v", got)
	}
	if got["collection_name"] != "kb_physical_one" {
		t.Fatalf("collection_name = %q", got["collection_name"])
	}
}
