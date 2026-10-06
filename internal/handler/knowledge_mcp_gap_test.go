package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/knowledge"
	knowledgeinfra "control-panel/internal/infrastructure/knowledge"

	"github.com/gin-gonic/gin"
)

func mcpCall(t *testing.T, router *gin.Engine, tool, args string) jsonRPCResponse {
	t.Helper()
	params := json.RawMessage(fmt.Sprintf(`{"name":%q,"arguments":%s}`, tool, args))
	rec := postJSONRPC(t, router, "tools/call", params, testValidToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("MCP status %d: %s", rec.Code, rec.Body.String())
	}
	var response jsonRPCResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("MCP response: %v", err)
	}
	return response
}

func TestKnowledgeMcpSearchScopeFailureHidesEarlierPages(t *testing.T) {
	called := 0
	svc := &fakeKnowledgeMcpService{retrievalFunc: func(_ context.Context, req knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error) {
		called++
		id := req["dataset_ids"].([]string)[0]
		if id == "kb-2" {
			id = "other-kb"
		}
		result := knowledge.RetrievalResult{"chunks": []any{map[string]any{"chunk_id": "c", "kb_id": id, "doc_id": "doc", "content": "private-first-page"}}}
		return &result, nil
	}}
	router := setupKnowledgeMcpRouter(svc, &fakeAgentMcpService{datasets: []string{"kb-1", "kb-2"}})
	resp := mcpCall(t, router, "knowledge_search", `{"query":"hello"}`)
	if called != 2 || !isErrorResult(resp.Result) || !strings.Contains(resultText(resp.Result), "[retrieval_scope_mismatch]") || strings.Contains(resultText(resp.Result), "private-first-page") {
		t.Fatalf("scope failure leaked prior page or was not detected: calls=%d result=%v", called, resp.Result)
	}
}

func TestKnowledgeMcpSearchExplicitEmptyAndNullScopes(t *testing.T) {
	called := 0
	svc := &fakeKnowledgeMcpService{retrievalFunc: func(_ context.Context, _ knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error) {
		called++
		return &knowledge.RetrievalResult{"chunks": []any{}}, nil
	}}
	router := setupKnowledgeMcpRouter(svc, &fakeAgentMcpService{datasets: []string{"kb-1"}})
	empty := mcpCall(t, router, "knowledge_search", `{"query":"hello","doc_ids":[]}`)
	if isErrorResult(empty.Result) || !strings.Contains(resultText(empty.Result), "范围为空") || called != 0 {
		t.Fatalf("empty document scope broadened: calls=%d result=%v", called, empty.Result)
	}
	for _, field := range []string{"dataset_ids", "doc_ids", "meta_data_filter", "reference_metadata_fields"} {
		resp := mcpCall(t, router, "knowledge_search", fmt.Sprintf(`{"query":"hello",%q:null}`, field))
		if !isErrorResult(resp.Result) || !strings.Contains(resultText(resp.Result), "[invalid_search_arguments]") || called != 0 {
			t.Fatalf("null %s scope broadened: calls=%d result=%v", field, called, resp.Result)
		}
	}
}

func TestKnowledgeMcpSearchForwardsVerifiedOptions(t *testing.T) {
	var got knowledge.RetrievalRequest
	svc := &fakeKnowledgeMcpService{retrievalFunc: func(_ context.Context, req knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error) {
		got = req
		return &knowledge.RetrievalResult{"chunks": []any{}}, nil
	}}
	router := setupKnowledgeMcpRouter(svc, &fakeAgentMcpService{datasets: []string{"kb-1"}})
	args := `{"query":"hello","dataset_ids":["kb-1"],"doc_ids":["doc-1"],"page":2,"page_size":5,"top_k":10,"search_mode":"hybrid","hybrid_dense_weight":0.8,"highlight":true,"reference_metadata_fields":["topic"],"meta_data_filter":{"method":"manual","logic":"and","manual":[{"key":"topic","op":"in","value":["guide",2,true]}]}}`
	resp := mcpCall(t, router, "knowledge_search", args)
	if isErrorResult(resp.Result) {
		t.Fatalf("valid search failed: %v", resp.Result)
	}
	mode, _ := got["search_mode"].(map[string]any)
	refs, _ := got["reference_metadata"].(map[string]any)
	if got["page"] != 2 || got["size"] != 5 || got["top_k"] != 10 || mode["type"] != "hybrid" || mode["weight_dense"] != 0.8 || got["highlight"] != true || refs["include"] != true || got["meta_data_filter"] == nil {
		t.Fatalf("search options lost: %#v", got)
	}
	bad := mcpCall(t, router, "knowledge_search", `{"query":"hello","meta_data_filter":{"method":"manual","manual":[{"key":"topic","op":"not in","value":["secret"]}]}}`)
	if !isErrorResult(bad.Result) || !strings.Contains(resultText(bad.Result), "[invalid_search_arguments]") {
		t.Fatalf("unsafe negative metadata filter accepted: %v", bad.Result)
	}
}

func TestKnowledgeMcpSearchShowsPerDatasetTotalWhenReturned(t *testing.T) {
	svc := &fakeKnowledgeMcpService{retrievalFunc: func(_ context.Context, _ knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error) {
		return &knowledge.RetrievalResult{"chunks": []any{}, "total": float64(27)}, nil
	}}
	router := setupKnowledgeMcpRouter(svc, &fakeAgentMcpService{datasets: []string{"kb-1"}})
	resp := mcpCall(t, router, "knowledge_search", `{"query":"hello","page":2}`)
	if isErrorResult(resp.Result) || !strings.Contains(resultText(resp.Result), "该库命中总数：27") {
		t.Fatalf("per-dataset pagination total missing: %v", resp.Result)
	}
}

func TestKnowledgeMcpChunksRejectDisabledAndMismatchedContent(t *testing.T) {
	for _, tc := range []struct {
		name         string
		status       string
		available    bool
		chunkDoc     string
		chunkDataset any
	}{
		{"disabled document", "0", true, "doc-1", "kb-1"},
		{"disabled chunk", "1", false, "doc-1", "kb-1"},
		{"wrong chunk document", "1", true, "doc-2", "kb-1"},
		{"wrong chunk dataset", "1", true, "doc-1", "other-kb"},
		{"invalid chunk dataset", "1", true, "doc-1", 123},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requested knowledge.ChunkListRequest
			svc := &fakeKnowledgeMcpService{listChunksFunc: func(_ context.Context, _, _ string, req knowledge.ChunkListRequest) (*knowledge.ChunkListResult, error) {
				requested = req
				return &knowledge.ChunkListResult{Document: knowledge.Document{"id": "doc-1", "dataset_id": "kb-1", "status": tc.status}, Chunks: []knowledge.Chunk{{"id": "chunk-1", "document_id": tc.chunkDoc, "dataset_id": tc.chunkDataset, "available": tc.available, "content": "hidden-text"}}}, nil
			}}
			router := setupKnowledgeMcpRouter(svc, &fakeAgentMcpService{datasets: []string{"kb-1"}})
			resp := mcpCall(t, router, "knowledge_chunks", `{"dataset_id":"kb-1","document_id":"doc-1","chunk_id":"chunk-1"}`)
			if requested.Available == nil || !*requested.Available || requested.ID != "chunk-1" || !isErrorResult(resp.Result) || strings.Contains(resultText(resp.Result), "hidden-text") {
				t.Fatalf("content boundary failed: request=%+v response=%v", requested, resp.Result)
			}
		})
	}
}

func TestKnowledgeMcpChunksAcceptLegacyChunkWithoutDatasetID(t *testing.T) {
	svc := &fakeKnowledgeMcpService{listChunksFunc: func(_ context.Context, datasetID, documentID string, req knowledge.ChunkListRequest) (*knowledge.ChunkListResult, error) {
		if datasetID != "kb-1" || documentID != "doc-1" || req.Available == nil || !*req.Available {
			t.Fatalf("unexpected scoped chunk request: dataset=%q document=%q request=%+v", datasetID, documentID, req)
		}
		return &knowledge.ChunkListResult{
			Document: knowledge.Document{"id": "doc-1", "dataset_id": "kb-1", "status": "1"},
			Chunks:   []knowledge.Chunk{{"id": "chunk-1", "document_id": "doc-1", "dataset_id": nil, "available": true, "content": "legacy-content"}},
		}, nil
	}}
	router := setupKnowledgeMcpRouter(svc, &fakeAgentMcpService{datasets: []string{"kb-1"}})
	resp := mcpCall(t, router, "knowledge_chunks", `{"dataset_id":"kb-1","document_id":"doc-1"}`)
	if isErrorResult(resp.Result) || !strings.Contains(resultText(resp.Result), "legacy-content") {
		t.Fatalf("legacy scoped chunk should remain readable: %v", resp.Result)
	}
}

func TestKnowledgeMcpRerankModelsOnlyEnabledAndSearchValidated(t *testing.T) {
	var got knowledge.RetrievalRequest
	svc := &fakeKnowledgeMcpService{retrievalFunc: func(_ context.Context, req knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error) {
		got = req
		return &knowledge.RetrievalResult{"chunks": []any{}}, nil
	}}
	modelSource := &stubMultiRAGMyLLMs{data: []byte(`{"Factory":{"llm":[{"type":"rerank","name":"rerank-1","status":"1"},{"type":"rerank","name":"off","status":"0"},{"type":"embedding","name":"embedding-1","status":"1"}]}}`)}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/knowledge/mcp", testAgentAuthMiddleware(testValidToken), NewKnowledgeMcpHandler(svc, &fakeAgentMcpService{datasets: []string{"kb-1"}}, modelSource).HandleMessage)
	models := mcpCall(t, router, "knowledge_rerank_models", `{}`)
	if isErrorResult(models.Result) || !strings.Contains(resultText(models.Result), "rerank-1@Factory") || strings.Contains(resultText(models.Result), "off@Factory") {
		t.Fatalf("model catalogue includes disabled model: %v", models.Result)
	}
	resp := mcpCall(t, router, "knowledge_search", `{"query":"hello","rerank_id":"rerank-1@Factory"}`)
	if isErrorResult(resp.Result) || got["rerank_id"] != "rerank-1@Factory" {
		t.Fatalf("enabled rerank not forwarded: %#v response=%v", got, resp.Result)
	}
	got = nil
	resp = mcpCall(t, router, "knowledge_search", `{"query":"hello","rerank_id":"off@Factory"}`)
	if !isErrorResult(resp.Result) || !strings.Contains(resultText(resp.Result), "[rerank_unavailable]") || got != nil {
		t.Fatalf("disabled rerank accepted: %#v response=%v", got, resp.Result)
	}
}

func TestKnowledgeMcpJSONRPCThroughGoMultiRAGClient(t *testing.T) {
	paths := make([]string, 0, 2)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/search") {
			t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("upstream body: %v", err)
		}
		ids, _ := body["dataset_ids"].([]any)
		if len(ids) != 1 || !strings.Contains(r.URL.Path, ids[0].(string)) {
			t.Errorf("search not isolated per KB: path=%s body=%v", r.URL.Path, body)
		}
		filter, _ := body["meta_data_filter"].(map[string]any)
		manual, _ := filter["manual"].([]any)
		if len(manual) != 1 {
			t.Errorf("metadata filter lost: %v", body)
		} else if condition, ok := manual[0].(map[string]any); !ok {
			t.Errorf("invalid metadata condition: %v", manual[0])
		} else if values, ok := condition["value"].([]any); !ok || len(values) != 3 || values[0] != "guide" || values[1] != float64(2) || values[2] != true {
			t.Errorf("typed membership changed: %v", condition["value"])
		}
		fmt.Fprintf(w, `{"retcode":0,"data":{"chunks":[{"chunk_id":"chunk-%s","kb_id":%q,"doc_id":"doc-1","docnm_kwd":"guide.pdf","content_with_weight":"answer","similarity":0.9}]}}`, ids[0], ids[0])
	}))
	defer remote.Close()
	engine := knowledgeinfra.NewRemoteMultiragEngine(remote.URL, "test-key", time.Second, time.Second)
	svc := services.NewKnowledgeService(engine, nil)
	router := setupKnowledgeMcpRouter(svc, &fakeAgentMcpService{datasets: []string{"kb-1", "kb-2"}})
	resp := mcpCall(t, router, "knowledge_search", `{"query":"hello","meta_data_filter":{"method":"manual","logic":"and","manual":[{"key":"topic","op":"in","value":["guide",2,true]}]}}`)
	if isErrorResult(resp.Result) || len(paths) != 2 {
		t.Fatalf("MCP to Go to search failed: paths=%v response=%v", paths, resp.Result)
	}
	for _, want := range []string{"/api/v1/datasets/kb-1/search", "/api/v1/datasets/kb-2/search", "分块ID：chunk-kb-1", "分块ID：chunk-kb-2"} {
		if strings.HasPrefix(want, "/") {
			if !strings.Contains(strings.Join(paths, " "), want) {
				t.Errorf("missing path %s: %v", want, paths)
			}
		} else if !strings.Contains(resultText(resp.Result), want) {
			t.Errorf("missing source %s: %s", want, resultText(resp.Result))
		}
	}
}
