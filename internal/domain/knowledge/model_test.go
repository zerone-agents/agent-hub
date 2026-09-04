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

func TestNormalizeRetrievalChunk_PromotesDocnmKwdToDocumentName(t *testing.T) {
	chunk := NormalizeRetrievalChunk(map[string]any{
		"chunk_id":            "ck-1",
		"content_with_weight": "术前血小板评估",
		"doc_id":              "doc-1",
		"docnm_kwd":           "PTNB外部指南与共识库_V2.pdf",
		"similarity":          0.87,
	})
	got := map[string]any(chunk)
	if got["document_name"] != "PTNB外部指南与共识库_V2.pdf" {
		t.Fatalf("document_name = %v, want promoted docnm_kwd", got["document_name"])
	}
	if _, ok := got["docnm_kwd"]; ok {
		t.Fatalf("docnm_kwd should be consumed: %#v", got)
	}
	if got["document_id"] != "doc-1" || got["id"] != "ck-1" || got["content"] != "术前血小板评估" {
		t.Fatalf("plain chunk mapping lost: %#v", got)
	}
}

func TestNormalizeRetrievalChunk_DocumentKeywordFallback(t *testing.T) {
	chunk := NormalizeRetrievalChunk(map[string]any{
		"doc_id":           "doc-2",
		"document_keyword": "共识库V2",
	})
	got := map[string]any(chunk)
	if got["document_name"] != "共识库V2" {
		t.Fatalf("document_name = %v, want document_keyword fallback", got["document_name"])
	}
	if _, ok := got["document_keyword"]; ok {
		t.Fatalf("document_keyword should be consumed: %#v", got)
	}
}

func TestNormalizeRetrievalChunk_ExistingDocumentNameWins(t *testing.T) {
	chunk := NormalizeRetrievalChunk(map[string]any{
		"document_name": "已有名称",
		"docnm_kwd":     "别名保留",
	})
	got := map[string]any(chunk)
	if got["document_name"] != "已有名称" {
		t.Fatalf("document_name = %v, want existing value", got["document_name"])
	}
	if got["docnm_kwd"] != "别名保留" {
		t.Fatalf("unconsumed alias must pass through: %#v", got)
	}
}
