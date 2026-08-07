package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type ProbeConfig struct {
	TransportType string
	URL           string
	Headers       map[string]string
}

type ProbeResult struct {
	Status string    `json:"status"`
	Tools  []McpTool `json:"tools,omitempty"`
	Error  string    `json:"error,omitempty"`
}

type McpTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ProbeClient struct {
	HTTPClient *http.Client
}

func NewProbeClient() *ProbeClient {
	return &ProbeClient{
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *ProbeClient) Probe(ctx context.Context, cfg ProbeConfig) (*ProbeResult, error) {
	if cfg.TransportType != "http" {
		return &ProbeResult{Status: "unsupported", Error: "SSE transport 探测暂不支持"}, nil
	}
	if cfg.URL == "" {
		return &ProbeResult{Status: "failed", Error: "URL 不能为空"}, nil
	}

	// 发送 initialize 并提取 Mcp-Session-Id
	sessionID, err := c.sendInitialize(ctx, cfg)
	if err != nil {
		return &ProbeResult{Status: "failed", Error: fmt.Sprintf("initialize 失败: %v", err)}, nil
	}

	// 发送 notifications/initialized（带 session header）
	if err := c.sendInitializedNotification(ctx, cfg, sessionID); err != nil {
		return &ProbeResult{Status: "failed", Error: fmt.Sprintf("notifications/initialized 失败: %v", err)}, nil
	}

	// 发送 tools/list（带 session header）
	tools, err := c.sendToolsList(ctx, cfg, sessionID)
	if err != nil {
		return &ProbeResult{Status: "failed", Error: fmt.Sprintf("tools/list 失败: %v", err)}, nil
	}
	return &ProbeResult{Status: "success", Tools: tools}, nil
}

func (c *ProbeClient) sendInitialize(ctx context.Context, cfg ProbeConfig) (string, error) {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "zerone-control-panel",
				"version": "0.2.0",
			},
		},
	}
	resp, headers, err := c.postJSONWithHeaders(ctx, cfg, payload)
	if err != nil {
		return "", err
	}
	if err := validateResponse(resp, "initialize"); err != nil {
		return "", err
	}
	// 提取 Mcp-Session-Id（可选，某些 MCP 服务可能不返回）
	sessionID := headers.Get("Mcp-Session-Id")
	return sessionID, nil
}

func (c *ProbeClient) sendInitializedNotification(ctx context.Context, cfg ProbeConfig, sessionID string) error {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	}
	_, _, err := c.postJSONWithHeaders(ctx, cfg, payload, sessionID)
	return err
}

func (c *ProbeClient) sendToolsList(ctx context.Context, cfg ProbeConfig, sessionID string) ([]McpTool, error) {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
	}
	resp, _, err := c.postJSONWithHeaders(ctx, cfg, payload, sessionID)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Result struct {
			Tools []McpTool `json:"tools"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(resp, &wrapper); err != nil {
		return nil, fmt.Errorf("解析 tools/list 响应失败: %w", err)
	}
	if wrapper.Error != nil {
		return nil, fmt.Errorf("MCP 返回错误: %s", wrapper.Error.Message)
	}
	return wrapper.Result.Tools, nil
}

func (c *ProbeClient) postJSONWithHeaders(ctx context.Context, cfg ProbeConfig, payload interface{}, sessionID ...string) ([]byte, http.Header, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}
	// 如果有 sessionID，添加到请求头
	if len(sessionID) > 0 && sessionID[0] != "" {
		req.Header.Set("Mcp-Session-Id", sessionID[0])
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	return extractSSEData(raw), resp.Header, nil
}

// extractSSEData extracts JSON from SSE format (data:{...}) or returns raw if plain JSON.
func extractSSEData(raw []byte) []byte {
	s := string(raw)
	// Fast path: plain JSON response
	if len(s) > 0 && s[0] == '{' {
		return raw
	}
	// SSE format: look for data: lines
	for _, line := range splitLines(s) {
		if len(line) > 5 && line[:5] == "data:" {
			return []byte(line[5:])
		}
	}
	return raw
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func validateResponse(body []byte, method string) error {
	var envelope struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("解析 %s 响应失败: %w", method, err)
	}
	if envelope.Error != nil {
		return fmt.Errorf("MCP 返回错误: %s", envelope.Error.Message)
	}
	return nil
}
