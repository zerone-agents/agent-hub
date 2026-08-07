package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/domain/provider"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// stubMultiRAGMyLLMs is a minimal MultiRAGMyLLMsSource that returns canned
// my_llms data (or an error).
type stubMultiRAGMyLLMs struct {
	data []byte
	err  error
}

func (s *stubMultiRAGMyLLMs) ListMyLLMs(ctx context.Context) (json.RawMessage, error) {
	if s.err != nil {
		return nil, s.err
	}
	return json.RawMessage(s.data), nil
}

// compile-time assertion that the stub satisfies the interface.
var _ provider.MultiRAGMyLLMsSource = (*stubMultiRAGMyLLMs)(nil)

// newMultiRAGModelsRouter builds a gin router with the ListMultiRAGModels
// route bound, for handler-level integration tests.
func newMultiRAGModelsRouter(h *KnowledgeHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/admin/knowledge/multirag/models", h.ListMultiRAGModels)
	return r
}

// sampleMyLLMsData has two factories: OpenAI (1 chat + 1 embedding) and
// MinerU (1 ocr). Used across multiple tests.
func sampleMyLLMsData() []byte {
	return []byte(`{
		"OpenAI": {"llm": [
			{"type": "chat", "name": "gpt-4o", "status": "1"},
			{"type": "embedding", "name": "text-embedding-3", "status": "1"}
		]},
		"MinerU": {"llm": [
			{"type": "ocr", "name": "mineru-custom-1", "status": "1"}
		]}
	}`)
}

func TestKnowledgeMultiRAGModels_HandlerReturnsFlatEmbeddingList(t *testing.T) {
	h := &KnowledgeHandler{
		multiragMyLLMs: &stubMultiRAGMyLLMs{data: sampleMyLLMsData()},
	}
	r := newMultiRAGModelsRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/knowledge/multirag/models?type=embedding", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Success bool `json:"success"`
		Data    []struct {
			Name    string `json:"name"`
			Factory string `json:"factory"`
			Type    string `json:"type"`
			Status  string `json:"status"`
			FullID  string `json:"fullId"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Len(t, resp.Data, 1)
	require.Equal(t, "text-embedding-3", resp.Data[0].Name)
	require.Equal(t, "OpenAI", resp.Data[0].Factory)
	require.Equal(t, "embedding", resp.Data[0].Type)
	require.Equal(t, "1", resp.Data[0].Status)
	require.Equal(t, "text-embedding-3@OpenAI", resp.Data[0].FullID)
}

func TestKnowledgeMultiRAGModels_FilterByOCR(t *testing.T) {
	h := &KnowledgeHandler{
		multiragMyLLMs: &stubMultiRAGMyLLMs{data: sampleMyLLMsData()},
	}
	r := newMultiRAGModelsRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/knowledge/multirag/models?type=ocr", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Success bool `json:"success"`
		Data    []struct {
			Name    string `json:"name"`
			Factory string `json:"factory"`
			Type    string `json:"type"`
			FullID  string `json:"fullId"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Len(t, resp.Data, 1)
	require.Equal(t, "mineru-custom-1", resp.Data[0].Name)
	require.Equal(t, "MinerU", resp.Data[0].Factory)
	require.Equal(t, "ocr", resp.Data[0].Type)
	require.Equal(t, "mineru-custom-1@MinerU", resp.Data[0].FullID)
}

func TestKnowledgeMultiRAGModels_TypeFilterIsCaseInsensitive(t *testing.T) {
	h := &KnowledgeHandler{
		multiragMyLLMs: &stubMultiRAGMyLLMs{data: sampleMyLLMsData()},
	}
	r := newMultiRAGModelsRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/knowledge/multirag/models?type=EMBEDDING", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data []struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 1)
	require.Equal(t, "text-embedding-3", resp.Data[0].Name)
}

func TestKnowledgeMultiRAGModels_MissingTypeReturns400(t *testing.T) {
	h := &KnowledgeHandler{
		multiragMyLLMs: &stubMultiRAGMyLLMs{data: sampleMyLLMsData()},
	}
	r := newMultiRAGModelsRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/knowledge/multirag/models", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestKnowledgeMultiRAGModels_UnconfiguredReturns503(t *testing.T) {
	h := &KnowledgeHandler{multiragMyLLMs: nil}
	r := newMultiRAGModelsRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/knowledge/multirag/models?type=embedding", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestKnowledgeMultiRAGModels_SourceErrorReturns503(t *testing.T) {
	h := &KnowledgeHandler{
		multiragMyLLMs: &stubMultiRAGMyLLMs{err: errors.New("upstream timeout")},
	}
	r := newMultiRAGModelsRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/knowledge/multirag/models?type=embedding", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestKnowledgeMultiRAGModels_EmptyResultReturnsEmptyArray(t *testing.T) {
	h := &KnowledgeHandler{
		multiragMyLLMs: &stubMultiRAGMyLLMs{data: []byte(`{}`)},
	}
	r := newMultiRAGModelsRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/knowledge/multirag/models?type=embedding", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Success bool `json:"success"`
		Data    []struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Empty(t, resp.Data)
}

// TestKnowledgeMultiRAGModels_ResponseIsSortedDeterministically seeds 3
// factories with multiple embedding models each and asserts the response is
// sorted by (Factory, Name). Without the sort, iterating the Go map would
// produce random order on every run.
func TestKnowledgeMultiRAGModels_ResponseIsSortedDeterministically(t *testing.T) {
	// Three factories chosen so lexicographic order differs from insertion
	// order: ZHIPU, Anthropic, OpenAI -> sorted: Anthropic, OpenAI, ZHIPU.
	// Within each factory, models are also out of order so name-sort is exercised.
	data := []byte(`{
		"ZHIPU-AI": {"llm": [
			{"type": "embedding", "name": "embedding-3", "status": "1"},
			{"type": "embedding", "name": "embedding-2", "status": "1"}
		]},
		"Anthropic": {"llm": [
			{"type": "embedding", "name": "claude-emb-b", "status": "1"},
			{"type": "embedding", "name": "claude-emb-a", "status": "1"}
		]},
		"OpenAI": {"llm": [
			{"type": "embedding", "name": "text-embedding-3-small", "status": "1"},
			{"type": "embedding", "name": "text-embedding-3-large", "status": "1"}
		]}
	}`)
	h := &KnowledgeHandler{multiragMyLLMs: &stubMultiRAGMyLLMs{data: data}}
	r := newMultiRAGModelsRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/knowledge/multirag/models?type=embedding", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Success bool `json:"success"`
		Data    []struct {
			Name    string `json:"name"`
			Factory string `json:"factory"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Len(t, resp.Data, 6)

	expected := []struct {
		Factory string
		Name    string
	}{
		{"Anthropic", "claude-emb-a"},
		{"Anthropic", "claude-emb-b"},
		{"OpenAI", "text-embedding-3-large"},
		{"OpenAI", "text-embedding-3-small"},
		{"ZHIPU-AI", "embedding-2"},
		{"ZHIPU-AI", "embedding-3"},
	}
	for i, want := range expected {
		require.Equalf(t, want.Factory, resp.Data[i].Factory, "position %d factory", i)
		require.Equalf(t, want.Name, resp.Data[i].Name, "position %d name", i)
	}
}
