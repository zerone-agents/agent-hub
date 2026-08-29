package kong

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Service represents a Kong Service object.
//
// Timeout fields are in milliseconds (Kong convention). Defaults when omitted
// from the Admin API are 60000ms — too short for SSE streams that can sit
// silent for minutes during long tool calls (e.g. WriteTool on a large file),
// so callers that need streaming should set ReadTimeout/WriteTimeout
// explicitly.
type Service struct {
	ID             string   `json:"id,omitempty"`
	Name           string   `json:"name"`
	Protocol       string   `json:"protocol"`
	Host           string   `json:"host"`
	Port           int      `json:"port"`
	ConnectTimeout int      `json:"connect_timeout,omitempty"`
	ReadTimeout    int      `json:"read_timeout,omitempty"`
	WriteTimeout   int      `json:"write_timeout,omitempty"`
	Tags           []string `json:"tags,omitempty"`
}

// ServiceRef is a nested reference to a Service, used inside Route payloads.
type ServiceRef struct {
	ID string `json:"id"`
}

// Route represents a Kong Route object.
type Route struct {
	ID                string      `json:"id,omitempty"`
	Name              string      `json:"name"`
	Hosts             []string    `json:"hosts"`
	Paths             []string    `json:"paths"`
	StripPath         bool        `json:"strip_path"`
	RequestBuffering  *bool       `json:"request_buffering,omitempty"`
	ResponseBuffering *bool       `json:"response_buffering,omitempty"`
	Service           *ServiceRef `json:"service,omitempty"`
	Tags              []string    `json:"tags,omitempty"`
}

// Client is a thin HTTP wrapper around the Kong Admin API.
// A nil Client is a valid no-op sentinel: callers that have no Kong backend
// configured (NewClient received an empty adminURL) get back nil and may use
// that to short-circuit gateway operations.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient returns a Client for the given Kong Admin URL.
// An empty adminURL returns nil so callers can treat the absence of Kong as a
// no-op rather than special-casing an error path.
func NewClient(adminURL string) *Client {
	if adminURL == "" {
		return nil
	}
	return &Client{
		baseURL: adminURL,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// GetService returns (svc, found, err). HTTP 404 maps to found=false with a
// nil error, allowing callers to distinguish "does not exist" from transport
// failures.
func (c *Client) GetService(ctx context.Context, name string) (*Service, bool, error) {
	var s Service
	found, err := c.get(ctx, "/services/"+url.PathEscape(name), &s)
	if err != nil || !found {
		return nil, found, err
	}
	return &s, true, nil
}

// CreateService creates a new Service via POST /services.
func (c *Client) CreateService(ctx context.Context, s *Service) (*Service, error) {
	var out Service
	if err := c.post(ctx, "/services", s, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateService updates an existing Service via PATCH /services/{name}.
func (c *Client) UpdateService(ctx context.Context, name string, s *Service) (*Service, error) {
	var out Service
	if err := c.patch(ctx, "/services/"+url.PathEscape(name), s, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteService deletes a Service. Kong cascades the delete to Routes attached
// to the Service. HTTP 404 is treated as success (idempotent deregistration).
func (c *Client) DeleteService(ctx context.Context, name string) error {
	return c.deleteIdempotent(ctx, "/services/"+url.PathEscape(name))
}

// ListServicesByTag lists Services tagged with the given tag, used for
// reconciliation of managed gateway objects. Kong paginates list endpoints
// via the offset cursor; the loop follows it until the response omits one.
func (c *Client) ListServicesByTag(ctx context.Context, tag string) ([]Service, error) {
	var out []Service
	offset := ""
	seen := map[string]bool{}
	for {
		path := "/services?tags=" + url.QueryEscape(tag)
		if offset != "" {
			path += "&offset=" + url.QueryEscape(offset)
		}
		var wrap struct {
			Data   []Service `json:"data"`
			Offset string    `json:"offset"`
		}
		if err := c.getRaw(ctx, path, &wrap); err != nil {
			return nil, err
		}
		if len(wrap.Data) == 0 && offset != "" {
			break // 防御：服务端异常回环时终止
		}
		out = append(out, wrap.Data...)
		if wrap.Offset == "" {
			break
		}
		if seen[wrap.Offset] {
			return nil, fmt.Errorf("kong: pagination cursor cycle detected at offset %q", wrap.Offset)
		}
		seen[wrap.Offset] = true
		offset = wrap.Offset
	}
	return out, nil
}

// ListRoutesByTag returns all routes carrying the given tag, following the
// offset pagination cursor to the last page.
func (c *Client) ListRoutesByTag(ctx context.Context, tag string) ([]Route, error) {
	var out []Route
	offset := ""
	seen := map[string]bool{}
	for {
		path := "/routes?tags=" + url.QueryEscape(tag)
		if offset != "" {
			path += "&offset=" + url.QueryEscape(offset)
		}
		var wrap struct {
			Data   []Route `json:"data"`
			Offset string  `json:"offset"`
		}
		if err := c.getRaw(ctx, path, &wrap); err != nil {
			return nil, err
		}
		if len(wrap.Data) == 0 && offset != "" {
			break // 防御：服务端异常回环时终止
		}
		out = append(out, wrap.Data...)
		if wrap.Offset == "" {
			break
		}
		if seen[wrap.Offset] {
			return nil, fmt.Errorf("kong: pagination cursor cycle detected at offset %q", wrap.Offset)
		}
		seen[wrap.Offset] = true
		offset = wrap.Offset
	}
	return out, nil
}

// GetRoute returns (route, found, err) with the same 404 semantics as
// GetService.
func (c *Client) GetRoute(ctx context.Context, name string) (*Route, bool, error) {
	var r Route
	found, err := c.get(ctx, "/routes/"+url.PathEscape(name), &r)
	if err != nil || !found {
		return nil, found, err
	}
	return &r, true, nil
}

// CreateRoute creates a new Route via POST /routes.
func (c *Client) CreateRoute(ctx context.Context, r *Route) (*Route, error) {
	var out Route
	if err := c.post(ctx, "/routes", r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateRoute updates an existing Route via PATCH /routes/{name}.
func (c *Client) UpdateRoute(ctx context.Context, name string, r *Route) (*Route, error) {
	var out Route
	if err := c.patch(ctx, "/routes/"+url.PathEscape(name), r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteRoute deletes a Route. HTTP 404 is treated as success (idempotent).
func (c *Client) DeleteRoute(ctx context.Context, name string) error {
	return c.deleteIdempotent(ctx, "/routes/"+url.PathEscape(name))
}

// get performs a single-object GET. HTTP 404 → (false, nil); other non-2xx →
// error with the truncated body for diagnostics.
func (c *Client) get(ctx context.Context, path string, out interface{}) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return false, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return false, fmt.Errorf("kong GET %s: HTTP %d %s", path, resp.StatusCode, string(body))
	}
	return true, json.NewDecoder(resp.Body).Decode(out)
}

// getRaw performs a GET for list endpoints, whose response is shaped as
// {"data":[...]}. It does not special-case 404 since list queries do not
// address a single resource.
func (c *Client) getRaw(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("kong GET %s: HTTP %d %s", path, resp.StatusCode, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) post(ctx context.Context, path string, body interface{}, out interface{}) error {
	return c.sendJSON(ctx, http.MethodPost, path, body, out)
}

func (c *Client) patch(ctx context.Context, path string, body interface{}, out interface{}) error {
	return c.sendJSON(ctx, http.MethodPatch, path, body, out)
}

// sendJSON marshals body as JSON and dispatches the request, decoding the
// response into out when non-nil.
func (c *Client) sendJSON(ctx context.Context, method, path string, body interface{}, out interface{}) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("kong %s %s: HTTP %d %s", method, path, resp.StatusCode, string(b))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// deleteIdempotent performs a DELETE; HTTP 404 is swallowed so that
// deregistration is safe to retry against a missing resource.
func (c *Client) deleteIdempotent(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("kong DELETE %s: HTTP %d %s", path, resp.StatusCode, string(body))
	}
	return nil
}
