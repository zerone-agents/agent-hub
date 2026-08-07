package services

import (
	"testing"
)

func TestConvertSession_PassesThroughModelSelectionID(t *testing.T) {
	in := SessionInput{
		ID:               "sess-1",
		CreatedAt:        "2026-07-18T10:00:00Z",
		UpdatedAt:        "2026-07-18T10:00:00Z",
		Model:            "kimi-k3",
		ModelSelectionID: "kimi-k3-2",
	}

	sess, err := convertSession(in)
	if err != nil {
		t.Fatalf("convertSession error: %v", err)
	}
	if sess.Model != "kimi-k3" {
		t.Errorf("Model: got %q, want %q", sess.Model, "kimi-k3")
	}
	if sess.ModelSelectionID != "kimi-k3-2" {
		t.Errorf("ModelSelectionID: got %q, want %q", sess.ModelSelectionID, "kimi-k3-2")
	}
}

func TestConvertSession_ModelSelectionIDOptional(t *testing.T) {
	// 老客户端不发送 model_selection_id 时应当解析成功，字段为空
	in := SessionInput{
		ID:        "sess-2",
		CreatedAt: "2026-07-18T10:00:00Z",
		UpdatedAt: "2026-07-18T10:00:00Z",
		Model:     "gpt-4",
	}
	sess, err := convertSession(in)
	if err != nil {
		t.Fatalf("convertSession error: %v", err)
	}
	if sess.ModelSelectionID != "" {
		t.Errorf("expected empty ModelSelectionID, got %q", sess.ModelSelectionID)
	}
}
