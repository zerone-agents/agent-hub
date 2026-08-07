package multirag

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClient_AddLLM_BareBoolTrue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`true`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "k")
	resp, err := c.AddLLM(context.Background(), AddLLMRequest{LLMFactory: "X", LLMName: "m", MdlType: "chat"})
	require.NoError(t, err)
	require.True(t, resp.Success, "literal `true` body must map to Success=true")
	require.Equal(t, "", resp.Message)
}

func TestClient_AddLLM_BareBoolFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`false`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "k")
	resp, err := c.AddLLM(context.Background(), AddLLMRequest{LLMFactory: "X", LLMName: "m", MdlType: "chat"})
	require.NoError(t, err)
	require.False(t, resp.Success, "literal `false` body must map to Success=false")
	require.Equal(t, "", resp.Message)
}

func TestClient_AddLLM_SuccessMessageShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success":true,"message":"ok"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "k")
	resp, err := c.AddLLM(context.Background(), AddLLMRequest{LLMFactory: "X", LLMName: "m", MdlType: "chat"})
	require.NoError(t, err)
	require.True(t, resp.Success)
	require.Equal(t, "ok", resp.Message)
}

func TestClient_AddLLM_MergesExtras(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/llm/add_llm", r.URL.Path)
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"retcode":0,"retmsg":"success","data":{}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "k")
	_, err := c.AddLLM(context.Background(), AddLLMRequest{
		LLMFactory: "Bedrock",
		LLMName:    "m",
		MdlType:    "chat",
		Extras: map[string]any{
			"auth_mode":      "access_key_secret",
			"bedrock_ak":     "AK",
			"bedrock_sk":     "SK",
			"bedrock_region": "us-east-1",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "Bedrock", capturedBody["llm_factory"])
	require.Equal(t, "access_key_secret", capturedBody["auth_mode"])
	require.Equal(t, "AK", capturedBody["bedrock_ak"])
	require.Equal(t, "us-east-1", capturedBody["bedrock_region"])
}

func TestClient_ListMyLLMs_EnvelopeSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/llm/my_llms", r.URL.Path)
		require.Equal(t, "include_details=true", r.URL.RawQuery)
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"retcode":0,"retmsg":"success","data":{"OpenAI":{"llm":[{"type":"chat","name":"gpt-4o","status":"1"}]}}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-key")
	raw, err := c.ListMyLLMs(context.Background())
	require.NoError(t, err)
	require.JSONEq(t, `{"OpenAI":{"llm":[{"type":"chat","name":"gpt-4o","status":"1"}]}}`, string(raw))
}

func TestClient_ListMyLLMs_NullDataSubstitutesEmptyObject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"retcode":0,"data":null}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "k")
	raw, err := c.ListMyLLMs(context.Background())
	require.NoError(t, err)
	require.Equal(t, `{}`, string(raw), "null data must be substituted with {} so downstream map unmarshal yields empty")
}

func TestClient_ListMyLLMs_NonZeroRetcode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"retcode":100,"retmsg":"unauthorized","data":null}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "k")
	_, err := c.ListMyLLMs(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "unauthorized")
}

func TestClient_ListMyLLMs_HTTP500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"detail":"internal server error"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "k")
	_, err := c.ListMyLLMs(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "500")
}

func TestClient_AddLLM_NestedAPIKeyObject(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"retcode":0,"retmsg":"success","data":{}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "k")
	nested := map[string]any{"mineru_apiserver": "http://x", "mineru_backend": "pipeline"}
	apiKeyJSON, _ := json.Marshal(nested)
	_, err := c.AddLLM(context.Background(), AddLLMRequest{
		LLMFactory: "MinerU", LLMName: "m", MdlType: "ocr",
		APIKey: apiKeyJSON,
	})
	require.NoError(t, err)

	apiKey, ok := capturedBody["api_key"].(map[string]any)
	require.True(t, ok, "api_key should be a nested object, not a string")
	require.Equal(t, "http://x", apiKey["mineru_apiserver"])
	require.Equal(t, "pipeline", apiKey["mineru_backend"])
}
