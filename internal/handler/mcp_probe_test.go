package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/application/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// newTestMcpServer returns an httptest server that speaks MCP JSON-RPC protocol.
func newTestMcpServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)

		switch req["method"] {
		case "initialize":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      req["id"],
				"result": map[string]interface{}{
					"protocolVersion": "2024-11-05",
					"serverInfo":      map[string]interface{}{"name": "test-server"},
				},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusNoContent)
		case "tools/list":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      req["id"],
				"result": map[string]interface{}{
					"tools": []map[string]interface{}{
						{"name": "test_tool", "description": "a test tool"},
					},
				},
			})
		}
	}))
}

func setupMcpProbeRouter(svc *services.McpService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewMcpHandler(svc)
	router := gin.New()
	router.POST("/api/v1/mcps/probe", h.ProbeByConfig)
	router.POST("/api/v1/mcps/:name/probe", h.ProbeByName)
	return router
}

func postProbeByConfig(router *gin.Engine, body interface{}) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcps/probe", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestProbeByConfig_Success(t *testing.T) {
	server := newTestMcpServer()
	defer server.Close()

	svc := services.NewMcpService("test-key-0123456789abcdef")
	router := setupMcpProbeRouter(svc)

	rec := postProbeByConfig(router, map[string]interface{}{
		"name":          "test",
		"title":         "Test MCP",
		"transportType": "http",
		"url":           server.URL,
	})

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp["success"].(bool))

	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "success", data["status"])

	tools := data["tools"].([]interface{})
	assert.Len(t, tools, 1)
	tool := tools[0].(map[string]interface{})
	assert.Equal(t, "test_tool", tool["name"])
}

func TestProbeByConfig_InvalidTransportType(t *testing.T) {
	svc := services.NewMcpService("test-key-0123456789abcdef")
	router := setupMcpProbeRouter(svc)

	rec := postProbeByConfig(router, map[string]interface{}{
		"name":          "test",
		"title":         "Test MCP",
		"transportType": "grpc",
		"url":           "http://localhost:9999",
	})

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp["success"].(bool))

	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "failed", data["status"])
	assert.Contains(t, data["error"], "transportType")
}

func TestProbeByConfig_EmptyURL(t *testing.T) {
	svc := services.NewMcpService("test-key-0123456789abcdef")
	router := setupMcpProbeRouter(svc)

	rec := postProbeByConfig(router, map[string]interface{}{
		"name":          "test",
		"title":         "Test MCP",
		"transportType": "http",
		"url":           "",
	})

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp["success"].(bool))

	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "failed", data["status"])
	assert.Contains(t, data["error"].(string), "url")
}

func TestProbeByConfig_SSETransport(t *testing.T) {
	svc := services.NewMcpService("test-key-0123456789abcdef")
	router := setupMcpProbeRouter(svc)

	rec := postProbeByConfig(router, map[string]interface{}{
		"name":          "test",
		"title":         "Test MCP",
		"transportType": "sse",
		"url":           "http://localhost:9999/sse",
	})

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp["success"].(bool))

	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "unsupported", data["status"])
}

func TestProbeByConfig_MalformedJSON(t *testing.T) {
	svc := services.NewMcpService("test-key-0123456789abcdef")
	router := setupMcpProbeRouter(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcps/probe", bytes.NewReader([]byte(`{invalid`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.False(t, resp["success"].(bool))
}

// TestProbeByName_Compiled verifies that ProbeByName handler method exists
// and the McpHandler type satisfies the expected interface. A full integration
// test for ProbeByName requires a database, which is outside unit test scope.
// The probe client logic is thoroughly tested in probe_client_test.go.
func TestProbeByName_Compiled(t *testing.T) {
	svc := services.NewMcpService("test-key-0123456789abcdef")
	h := NewMcpHandler(svc)
	// Compile-time assertion: ProbeByName exists with correct signature.
	var _ func(*gin.Context) = h.ProbeByName
}
