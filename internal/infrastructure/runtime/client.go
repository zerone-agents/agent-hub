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

	"control-panel/internal/domain/chat"
)

// HeaderExpectedContainerID is the runtime v2.7.0 expected-generation
// contract header (runtime issue #61 / hub issue #94): when set on an
// upload / file read / attachment-bearing run request, the runtime
// atomically verifies — before any upload write or file read — that its own
// container id matches the header value (constant-time compare; a container
// identified by hostname only accepts the full 64-hex id's first 12 chars).
// Mismatch → 412 generation_mismatch; runtime unable to determine its own
// identity → 503 generation_unavailable. An absent header skips the check
// (backward compatibility) — but hub-side callers MUST NOT strip the header
// and retry after a 412: that would silently reopen the TOCTOU window the
// contract closes.
const HeaderExpectedContainerID = "X-Expected-Container-Id"

// Client is an HTTP client for an open-agent-runtime container.
type Client struct {
	httpClient *http.Client
}

// RuntimeHTTPError marks a non-2xx runtime response. The upstream error body
// is deliberately NOT carried: runtime responses can echo the resolved agent
// config, whose MCP connection headers the hub armed at deploy time
// (X-Agent-Capability / Authorization, issue #111) — interpolating that body
// into an error that crosses the hub egress would leak the credentials past
// the content redaction (main #121 constraint, preserved). The only body
// derivative kept is an ALLOWLISTED attachment-domain code, parsed at the
// client boundary and discarded immediately.
type RuntimeHTTPError struct {
	StatusCode int
	// AttachmentCode is a stable attachment-domain code only (one of
	// chat.ErrCode*); empty for non-attachment errors. Never raw body text.
	AttachmentCode string
}

func (e *RuntimeHTTPError) Error() string {
	if e.AttachmentCode != "" {
		return fmt.Sprintf("runtime returned HTTP %d (attachment code %s)", e.StatusCode, e.AttachmentCode)
	}
	return fmt.Sprintf("runtime returned HTTP %d", e.StatusCode)
}

// allowlistedAttachmentCode extracts a stable attachment-domain code from a
// runtime error body and DISCARDS the body — the only string allowed to
// survive the client boundary (credential-leak constraint, main #121).
func allowlistedAttachmentCode(body string) string {
	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return ""
	}
	switch payload.Code {
	case chat.ErrCodeAttachmentMissing, chat.ErrCodeInvalidAttachment,
		chat.ErrCodeUploadLimitExceeded, chat.ErrCodeGenerationMismatch,
		chat.ErrCodeGenerationUnavailable:
		return payload.Code
	}
	return ""
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
// expectedContainerID: when non-empty, sent as X-Expected-Container-Id so the
// runtime atomically rejects (412/503) before processing if its container
// generation changed (runtime v2.7.0 contract). Empty omits the header
// (text-only runs: no generation to assert).
func (c *Client) StreamRun(ctx context.Context, baseURL, agentName, apiKey string, body []byte, expectedContainerID string) (io.ReadCloser, error) {
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
	if expectedContainerID != "" {
		req.Header.Set(HeaderExpectedContainerID, expectedContainerID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("runtime request failed: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// The body is parsed for an allowlisted attachment code and discarded
		// immediately — raw body text never crosses this boundary (main #121
		// credential-leak constraint).
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		resp.Body.Close()
		return nil, &RuntimeHTTPError{
			StatusCode:     resp.StatusCode,
			AttachmentCode: allowlistedAttachmentCode(string(buf)),
		}
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
		// Discard the upstream body (see RuntimeHTTPError): it may echo the
		// resolved MCP headers (capability / Authorization) which must never
		// cross the hub egress, even inside an error string.
		return nil, &RuntimeHTTPError{StatusCode: resp.StatusCode}
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
// expectedContainerID: when non-empty, sent as X-Expected-Container-Id for
// the runtime's atomic generation check (v2.7.0 contract); empty omits it.
func (c *Client) ProxyFiles(ctx context.Context, method, baseURL, apiKey, pathAndQuery string, rangeHeader string, expectedContainerID string) (*http.Response, error) {
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
	if expectedContainerID != "" {
		req.Header.Set(HeaderExpectedContainerID, expectedContainerID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("runtime proxy request failed: %w", err)
	}

	return resp, nil
}

// HealthInfo is the runtime GET /health response. The endpoint is
// unauthenticated and reports the runtime version (from its package.json)
// plus its capability declarations. Used for attachment capability probing
// only (issue #94): attachments require the runtime to declare
// attachmentExpectedGeneration (runtime >= 2.7.0, the release that added the
// X-Expected-Container-Id atomic generation check). A runtime without a
// capabilities field parses as false — the version string is informational
// only and no longer gates anything.
type HealthInfo struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	// Capabilities carries the runtime's feature declarations. Absent on
	// pre-2.7.0 runtimes → zero value → attachmentExpectedGeneration=false.
	Capabilities struct {
		AttachmentExpectedGeneration bool `json:"attachmentExpectedGeneration"`
	} `json:"capabilities"`
}

// Health calls GET {baseURL}/health. Capability probing only (issue #94):
// attachments require the runtime to declare attachmentExpectedGeneration.
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
// expectedContainerID: when non-empty, sent as X-Expected-Container-Id so
// the runtime rejects the upload with 412 before writing any file if its
// container generation changed (runtime v2.7.0 contract); empty omits it.
func (c *Client) UploadFiles(ctx context.Context, baseURL, apiKey string, body io.Reader, contentType string, expectedContainerID string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/files/uploads", body)
	if err != nil {
		return nil, fmt.Errorf("build upload request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}
	if expectedContainerID != "" {
		req.Header.Set(HeaderExpectedContainerID, expectedContainerID)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("runtime upload request failed: %w", err)
	}
	return resp, nil
}
