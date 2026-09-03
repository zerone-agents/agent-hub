// Package runtime provides an HTTP client for the open-agent-runtime
// containers spawned by agent-deployer. The client is intentionally thin:
// it exposes a single streaming method that returns the raw SSE byte stream
// for the caller to forward (flush-per-chunk) to its own HTTP response.
package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// RuntimeHTTPError is returned by client methods when the runtime responds
// with a non-2xx status. It carries the status and body so handlers can map
// runtime domain error codes (e.g. attachment_missing) to their own envelope.
type RuntimeHTTPError struct {
	Status int
	Body   string
}

func (e *RuntimeHTTPError) Error() string {
	return fmt.Sprintf("runtime returned HTTP %d: %s", e.Status, e.Body)
}

// Client is an HTTP client for an open-agent-runtime container.
type Client struct {
	httpClient *http.Client
}

// NewClient returns a runtime Client. The HTTP setup is tuned for SSE:
//   - No whole-request Timeout: a streaming run can legitimately last tens of
//     minutes across multiple tool calls. A blanket Timeout would kill the
//     stream mid-turn. The caller's request context is the real deadline.
//   - ResponseHeaderTimeout (30s): protects against a runtime that accepts the
//     TCP connection but never returns headers (hung container, blackholed
//     route). This is the only timeout that is safe to enforce for a stream.
//   - Idle connection reuse is inherited from DefaultTransport.
//
// A 30-minute hard ceiling is kept purely as a last-resort leak guard; real
// cancellation flows through the request context tied to the HTTP handler.
func NewClient() *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 30 * time.Second
	return &Client{
		httpClient: &http.Client{
			Timeout:   30 * time.Minute,
			Transport: transport,
		},
	}
}

// StreamRun POSTs to {baseURL}/v1/agents/{agentName}/runs and returns the
// response body for the caller to read SSE events from. The caller MUST
// close the returned reader when done.
//
// baseURL example: "http://47.116.174.109:32780"
// agentName: the agent identifier registered inside the runtime container
// apiKey: the runtime's ZERONE_AGENT_HTTP_API_KEY value, sent as x-api-key
// body: the JSON request body, e.g. {"message":"Hello"}
func (c *Client) StreamRun(ctx context.Context, baseURL, agentName, apiKey string, body []byte) (io.ReadCloser, error) {
	url := fmt.Sprintf("%s/v1/agents/%s/runs", baseURL, agentName)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build runtime request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("runtime request failed: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, &RuntimeHTTPError{Status: resp.StatusCode, Body: string(buf)}
	}

	return resp.Body, nil
}

// GetAgentDetail calls GET {baseURL}/v1/agents/{agentName} and returns the
// raw JSON response body. The caller (control-panel handler) forwards the
// bytes directly to its own HTTP response without re-wrapping, so the
// runtime's JSON shape (AgentDetail) is preserved end-to-end.
//
// On non-2xx runtime responses, the returned error contains the status code
// (e.g. "HTTP 404") so the caller can map it to an appropriate HTTP status.
func (c *Client) GetAgentDetail(ctx context.Context, baseURL, agentName, apiKey string) ([]byte, error) {
	url := fmt.Sprintf("%s/v1/agents/%s", baseURL, agentName)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build runtime detail request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("runtime detail request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB cap; AgentDetail is small JSON

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("runtime returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// ProxyFiles issues a request to {baseURL}/v1/files... with the given method,
// query string, and optional Range header, then returns the runtime's HTTP
// response untouched. The caller MUST close the returned response.
//
// Why a single method instead of three (ListFiles/GetContent/HeadContent):
// the three runtime endpoints differ only in path/query/method. Folding them
// into one proxy keeps the handler code small and avoids drift when runtime
// adds headers in the future.
//
// Business-level 4xx (400/404/416) are returned as a normal *http.Response
// (NOT as error) so the handler can map them with proper status preservation.
// Network failures are returned as error. Any HTTP status (4xx and 5xx) is
// returned as a normal *http.Response so the handler can map it with status
// preservation.
//
// method: "GET" or "HEAD"
// pathAndQuery: runtime path + full query string, e.g.
//
//	"/v1/files?path=src&recursive=true" or
//	"/v1/files/content?path=package.json"
//
// rangeHeader: empty string omits the header; non-empty is forwarded as-is
func (c *Client) ProxyFiles(ctx context.Context, method, baseURL, apiKey, pathAndQuery string, rangeHeader string) (*http.Response, error) {
	url := baseURL + pathAndQuery

	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build proxy request: %w", err)
	}
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("runtime proxy request failed: %w", err)
	}

	return resp, nil
}

// HealthInfo is the runtime GET /health response. The endpoint is
// unauthenticated and reports the runtime version (from its package.json)
// and the process uptime in seconds (used to derive the container boot
// time — issue #94 review R2 F1 deployment-generation binding).
type HealthInfo struct {
	Status  string  `json:"status"`
	Version string  `json:"version"`
	Uptime  float64 `json:"uptime"`
}

// Health calls GET {baseURL}/health. Capability probing only (issue #94):
// attachments require runtime >= 2.5.0.
func (c *Client) Health(ctx context.Context, baseURL string) (*HealthInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
	if err != nil {
		return nil, fmt.Errorf("build health request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("runtime health request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("runtime health returned HTTP %d", resp.StatusCode)
	}
	var info HealthInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("parse health response: %w", err)
	}
	return &info, nil
}

// UploadFiles POSTs a multipart body to {baseURL}/v1/files/uploads and
// returns the runtime's raw response. The caller MUST close the response.
// contentType must carry the multipart boundary VERBATIM from the writer
// that produced body (re-writing it breaks the runtime's parser).
func (c *Client) UploadFiles(ctx context.Context, baseURL, apiKey string, body io.Reader, contentType string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/files/uploads", body)
	if err != nil {
		return nil, fmt.Errorf("build upload request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("runtime upload request failed: %w", err)
	}
	return resp, nil
}
