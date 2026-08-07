package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProbeClient_Success(t *testing.T) {
	// httptest server that responds to initialize, notifications/initialized, tools/list
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify Accept header
		assert.Contains(t, r.Header.Get("Accept"), "text/event-stream")
		assert.Contains(t, r.Header.Get("Accept"), "application/json")

		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)

		if req["method"] == "initialize" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      req["id"],
				"result": map[string]interface{}{
					"protocolVersion": "2024-11-05",
					"serverInfo":      map[string]interface{}{"name": "test"},
				},
			})
			return
		}
		if req["method"] == "notifications/initialized" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if req["method"] == "tools/list" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      req["id"],
				"result": map[string]interface{}{
					"tools": []map[string]interface{}{
						{"name": "search", "description": "search the web"},
					},
				},
			})
			return
		}
	}))
	defer server.Close()

	client := NewProbeClient()
	result, err := client.Probe(context.Background(), ProbeConfig{
		TransportType: "http",
		URL:           server.URL,
		Headers:       map[string]string{},
	})

	assert.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	assert.Len(t, result.Tools, 1)
	assert.Equal(t, "search", result.Tools[0].Name)
}

func TestProbeClient_SSEResponse(t *testing.T) {
	// Simulate SSE-format responses (like BigModel MCP)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "text/event-stream")
		if req["method"] == "initialize" {
			fmt.Fprintf(w, "id:1\nevent:message\ndata:{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"protocolVersion\":\"2024-11-05\",\"serverInfo\":{\"name\":\"test\"}}}\n\n")
			return
		}
		if req["method"] == "notifications/initialized" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if req["method"] == "tools/list" {
			fmt.Fprintf(w, "id:1\nevent:message\ndata:{\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{\"tools\":[{\"name\":\"search\",\"description\":\"search the web\"}]}}\n\n")
			return
		}
	}))
	defer server.Close()

	client := NewProbeClient()
	result, err := client.Probe(context.Background(), ProbeConfig{
		TransportType: "http",
		URL:           server.URL,
		Headers:       map[string]string{},
	})

	assert.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	assert.Len(t, result.Tools, 1)
	assert.Equal(t, "search", result.Tools[0].Name)
}

func TestProbeClient_SSEUnsupported(t *testing.T) {
	client := NewProbeClient()
	result, err := client.Probe(context.Background(), ProbeConfig{
		TransportType: "sse",
		URL:           "http://localhost:9999/sse",
	})
	assert.NoError(t, err)
	assert.Equal(t, "unsupported", result.Status)
}

func TestProbeClient_EmptyURL(t *testing.T) {
	client := NewProbeClient()
	result, err := client.Probe(context.Background(), ProbeConfig{
		TransportType: "http",
		URL:           "",
	})
	assert.NoError(t, err)
	assert.Equal(t, "failed", result.Status)
	assert.Contains(t, result.Error, "URL")
}

func TestProbeClient_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewProbeClient()
	result, err := client.Probe(context.Background(), ProbeConfig{
		TransportType: "http",
		URL:           server.URL,
	})
	assert.NoError(t, err)
	assert.Equal(t, "failed", result.Status)
	assert.Contains(t, result.Error, "initialize 失败")
}

func TestProbeClient_McpSessionId(t *testing.T) {
	// Server that requires Mcp-Session-Id for subsequent requests
	var sessionID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)

		if req["method"] == "initialize" {
			// Return Mcp-Session-Id in response header
			sessionID = "test-session-123"
			w.Header().Set("Mcp-Session-Id", sessionID)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      req["id"],
				"result": map[string]interface{}{
					"protocolVersion": "2024-11-05",
					"serverInfo":      map[string]interface{}{"name": "test"},
				},
			})
			return
		}
		// Verify Mcp-Session-Id is passed in subsequent requests
		if req["method"] == "notifications/initialized" || req["method"] == "tools/list" {
			if r.Header.Get("Mcp-Session-Id") != sessionID {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}
		if req["method"] == "notifications/initialized" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if req["method"] == "tools/list" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      req["id"],
				"result": map[string]interface{}{
					"tools": []map[string]interface{}{
						{"name": "tool1", "description": "test tool"},
					},
				},
			})
			return
		}
	}))
	defer server.Close()

	client := NewProbeClient()
	result, err := client.Probe(context.Background(), ProbeConfig{
		TransportType: "http",
		URL:           server.URL,
		Headers:       map[string]string{},
	})

	assert.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	assert.Len(t, result.Tools, 1)
	assert.Equal(t, "tool1", result.Tools[0].Name)
}

func TestProbeClient_BadRequest(t *testing.T) {
	// Simulate server that rejects without proper Accept header (like BigModel)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  map[string]interface{}{"protocolVersion": "2024-11-05"},
		})
	}))
	defer server.Close()

	client := NewProbeClient()
	result, err := client.Probe(context.Background(), ProbeConfig{
		TransportType: "http",
		URL:           server.URL,
		Headers:       map[string]string{},
	})

	assert.NoError(t, err)
	assert.Equal(t, "success", result.Status)
}
