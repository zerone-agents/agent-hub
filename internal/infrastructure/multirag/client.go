// Package multirag provides an HTTP client for MultiRAG's /v1/llm/* and
// /v1/user/* endpoints. It is intentionally separate from the knowledge
// client (which targets dataset/chunk endpoints) to keep the two domains
// decoupled.
package multirag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"control-panel/internal/domain/provider"
)

// Type aliases so callers don't have to import the domain package.
type (
	AddLLMRequest    = provider.AddLLMRequest
	MultiRAGResponse = provider.MultiRAGResponse
)

// Client implements provider.MultiRAGClient over HTTP.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// NewClient constructs an HTTP MultiRAG client.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// AddLLM posts to /v1/llm/add_llm.
func (c *Client) AddLLM(ctx context.Context, payload AddLLMRequest) (*MultiRAGResponse, error) {
	return c.do(ctx, "/v1/llm/add_llm", payload)
}

func (c *Client) do(ctx context.Context, path string, payload any) (*MultiRAGResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	out := &MultiRAGResponse{HTTPStatusCode: resp.StatusCode, Raw: raw}

	// HTTP non-2xx -> surface as failure but not transport error.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		out.Success = false
		out.Message = extractErrorMessage(raw)
		return out, nil
	}

	// Bare bool first (some MultiRAG endpoints return literal `true`/`false`).
	// This must run BEFORE the envelope unmarshal, since `true`/`false` cannot
	// unmarshal into a struct and would otherwise fall through to the unknown
	// shape default below.
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		out.Success = b
		out.Message = ""
		return out, nil
	}

	// Try the standard envelope.
	var env struct {
		RetCode *int            `json:"retcode"`
		Code    *int            `json:"code"`
		RetMsg  string          `json:"retmsg"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err == nil {
		// retcode/code present -> standard envelope.
		if env.RetCode != nil || env.Code != nil {
			code := 0
			if env.RetCode != nil {
				code = *env.RetCode
			} else if env.Code != nil {
				code = *env.Code
			}
			out.Success = code == 0
			if env.RetMsg != "" {
				out.Message = env.RetMsg
			} else {
				out.Message = env.Message
			}
			return out, nil
		}
		// Success/message shape.
		if env.Message != "" || strings.Contains(string(raw), `"success"`) {
			var sm struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			_ = json.Unmarshal(raw, &sm)
			out.Success = sm.Success
			out.Message = sm.Message
			return out, nil
		}
	}

	// Unknown shape; default to success since HTTP was 2xx.
	out.Success = true
	return out, nil
}

// extractErrorMessage pulls a human-readable message from a non-2xx body.
// MultiRAG HTTP errors look like `{"detail": "..."}`.
func extractErrorMessage(raw []byte) string {
	var d struct {
		Detail  string `json:"detail"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(raw, &d); err == nil {
		if d.Detail != "" {
			return d.Detail
		}
		if d.Message != "" {
			return d.Message
		}
		if d.Error != "" {
			return d.Error
		}
	}
	s := strings.TrimSpace(string(raw))
	if len(s) > 256 {
		s = s[:256]
	}
	return s
}

// Compile-time interface assertion.
var _ provider.MultiRAGClient = (*Client)(nil)

// Compile-time assertion that Client also satisfies MultiRAGMyLLMsSource.
// Same Client instance serves both sync (add_llm) and the read-only my_llms
// proxy endpoint.
var _ provider.MultiRAGMyLLMsSource = (*Client)(nil)

// ListMyLLMs GETs /v1/llm/my_llms?include_details=true and returns the
// unwrapped `data` field. The shape is:
//
//	{ "<Factory>": { "llm": [ {type, name, status, ...} ] } }
//
// Returns an error if the HTTP call fails, the HTTP status is non-2xx, or
// the MultiRAG retcode is non-zero.
func (c *Client) ListMyLLMs(ctx context.Context) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/llm/my_llms?include_details=true", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("MultiRAG my_llms returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// Unwrap the standard MultiRAG envelope.
	var env struct {
		RetCode *int            `json:"retcode"`
		Code    *int            `json:"code"`
		RetMsg  string          `json:"retmsg"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("解析 my_llms envelope 失败: %w", err)
	}
	code := 0
	if env.RetCode != nil {
		code = *env.RetCode
	} else if env.Code != nil {
		code = *env.Code
	}
	if code != 0 {
		msg := env.RetMsg
		if msg == "" {
			msg = "MultiRAG my_llms error"
		}
		return nil, fmt.Errorf("MultiRAG my_llms error: %s", msg)
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		// Default to an empty object so downstream json.Unmarshal into a
		// map produces an empty result instead of nil.
		return json.RawMessage(`{}`), nil
	}
	return env.Data, nil
}
