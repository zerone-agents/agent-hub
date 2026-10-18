package handler

import (
	"context"
	"errors"
	"strings"
	"testing"

	"control-panel/internal/domain/knowledge"
)

func TestKnowledgeMcpJointCompleteSelectionAndTotal(t *testing.T) {
	calls := 0
	svc := &fakeKnowledgeMcpService{retrievalFunc: func(_ context.Context, req knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error) {
		calls++
		ids := req["dataset_ids"].([]string)
		if len(ids) != 2 || ids[0] != "kb-2" || ids[1] != "kb-1" {
			t.Fatalf("selection changed: %v", ids)
		}
		if req["similarity_threshold"] != 0.6 {
			t.Fatalf("threshold changed: %v", req)
		}
		return &knowledge.RetrievalResult{"total": 31, "chunks": []any{
			map[string]any{"id": "c2", "kb_id": "kb-2", "doc_id": "d2", "content": "second dataset"},
			map[string]any{"id": "c1", "kb_id": "kb-1", "doc_id": "d1", "content": "first dataset"},
		}}, nil
	}}
	router := setupKnowledgeMcpRouter(svc, &fakeAgentMcpService{datasets: []string{"kb-1", "kb-2", "kb-other"}})
	resp := mcpCall(t, router, "knowledge_search", `{"query":"hello","search_strategy":"joint","dataset_ids":["kb-2","kb-1"],"doc_ids":["d1","d2"],"similarity_threshold":0.6}`)
	text := resultText(resp.Result)
	if calls != 1 || isErrorResult(resp.Result) || !strings.Contains(text, "联合命中总数：31") || !strings.Contains(text, "分块ID：c2") || strings.Contains(text, "该库命中总数") || strings.Contains(text, "分别检索") {
		t.Fatalf("incorrect joint result: calls=%d %v", calls, resp.Result)
	}
}

func TestKnowledgeMcpJointFailureNeverFallsBack(t *testing.T) {
	calls := 0
	svc := &fakeKnowledgeMcpService{retrievalFunc: func(_ context.Context, req knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error) {
		calls++
		return nil, errors.New("embedding model mismatch")
	}}
	router := setupKnowledgeMcpRouter(svc, &fakeAgentMcpService{datasets: []string{"kb-1", "kb-2"}})
	resp := mcpCall(t, router, "knowledge_search", `{"query":"hello","search_strategy":"joint"}`)
	if calls != 1 || !isErrorResult(resp.Result) || !strings.Contains(resultText(resp.Result), "[retrieval_failed]") || !strings.Contains(resultText(resp.Result), "同一嵌入模型实例") {
		t.Fatalf("failure fell back: calls=%d %v", calls, resp.Result)
	}
}

func TestKnowledgeMcpJointAuthorizationEmptyAndStrategy(t *testing.T) {
	calls := 0
	svc := &fakeKnowledgeMcpService{retrievalFunc: func(_ context.Context, req knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error) {
		calls++
		return &knowledge.RetrievalResult{"chunks": []any{}}, nil
	}}
	router := setupKnowledgeMcpRouter(svc, &fakeAgentMcpService{datasets: []string{"kb-1"}})
	for _, args := range []string{
		`{"query":"q","search_strategy":"joint","dataset_ids":["other"]}`,
		`{"query":"q","search_strategy":"joint","dataset_ids":[]}`,
		`{"query":"q","search_strategy":"automatic"}`,
		`{"query":"q","search_strategy":null}`,
	} {
		if resp := mcpCall(t, router, "knowledge_search", args); !isErrorResult(resp.Result) {
			t.Fatalf("invalid request accepted: %s %v", args, resp.Result)
		}
	}
	empty := mcpCall(t, router, "knowledge_search", `{"query":"q","search_strategy":"joint","doc_ids":[]}`)
	if calls != 0 || isErrorResult(empty.Result) || !strings.Contains(resultText(empty.Result), "范围为空") {
		t.Fatalf("empty scope broadened: %d %v", calls, empty.Result)
	}
}

func TestKnowledgeMcpJointSourceBoundary(t *testing.T) {
	for _, bad := range []map[string]any{
		{"id": "c", "kb_id": "other", "doc_id": "d"},
		{"id": "c", "kb_id": "kb-1", "dataset_id": "other", "doc_id": "d"},
		{"id": "c", "kb_id": "kb-1", "doc_id": "other"},
		{"id": "c", "kb_id": "kb-1", "doc_id": "d", "document_id": "other"},
		{"id": "c", "chunk_id": "other", "kb_id": "kb-1", "doc_id": "d"},
		{"id": "c", "doc_id": "d"},
		{"id": "c", "kb_id": "kb-1", "doc_id": "d", "available": false},
	} {
		result := knowledge.RetrievalResult{"chunks": []any{bad}}
		if err := validateMcpSearchResult(&result, []string{"kb-1", "kb-2"}, &[]string{"d"}); err == nil {
			t.Fatalf("invalid source accepted: %v", bad)
		}
	}
	result := knowledge.RetrievalResult{"chunks": []any{}, "doc_aggs": []any{map[string]any{"doc_id": "foreign", "kb_id": "kb-1"}}}
	if err := validateMcpSearchResult(&result, []string{"kb-1"}, &[]string{"d"}); err == nil {
		t.Fatal("out-of-scope aggregate accepted")
	}
}
