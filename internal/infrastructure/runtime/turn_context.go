package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"
)

const MaxTurnContextRunes = 6000

var (
	ErrInvalidTurnContext     = errors.New("turnContext must be valid UTF-8 and at most 6000 Unicode code points")
	ErrTurnContextUnsupported = errors.New("runtime does not declare the temporary turnContext contract")
)

// TurnContextRunRequest is the business-facing run DTO. TurnContext is only
// mapped to the runtime's per-run appendSystemPrompt; it is never a message.
type TurnContextRunRequest struct {
	Message           string `json:"message"`
	SessionID         string `json:"sessionId,omitempty"`
	RunID             string `json:"runId,omitempty"`
	TurnContext       string `json:"turnContext,omitempty"`
	MaxSessionQueries *int   `json:"maxSessionQueries,omitempty"`
}

func MarshalTurnContextRun(req TurnContextRunRequest) ([]byte, error) {
	if !utf8.ValidString(req.TurnContext) || utf8.RuneCountInString(req.TurnContext) > MaxTurnContextRunes {
		return nil, ErrInvalidTurnContext
	}
	return json.Marshal(struct {
		Message            string `json:"message"`
		SessionID          string `json:"sessionId,omitempty"`
		RunID              string `json:"runId,omitempty"`
		AppendSystemPrompt string `json:"appendSystemPrompt,omitempty"`
		MaxSessionQueries  *int   `json:"maxSessionQueries,omitempty"`
	}{req.Message, req.SessionID, req.RunID, req.TurnContext, req.MaxSessionQueries})
}

// MemoryRuntimeCapabilities reports declarations, not live provider acceptance.
// Older runtimes can silently ignore unknown JSON fields, so a version string
// alone must not enable the business temporary-context or history contract.
type MemoryRuntimeCapabilities struct {
	Version           string `json:"version"`
	TurnContext       bool   `json:"turnContext"`
	SessionAppend     bool   `json:"sessionAppend"`
	SessionRollback   bool   `json:"sessionRollback"`
	ExplicitZeroTools bool   `json:"explicitZeroTools"`
}

func (c *Client) ProbeMemoryCapabilities(ctx context.Context, baseURL, apiKey string) (MemoryRuntimeCapabilities, error) {
	var result MemoryRuntimeCapabilities
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/health", nil)
	if err != nil {
		return result, err
	}
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return result, errors.New("runtime capability probe failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return result, &RuntimeHTTPError{StatusCode: resp.StatusCode}
	}
	var health struct {
		Version      string `json:"version"`
		Capabilities struct {
			TurnContext struct {
				Field         string `json:"field"`
				MaxCodePoints int    `json:"maxCodePoints"`
				Persistence   string `json:"persistence"`
			} `json:"turnContext"`
			History struct {
				Version     int    `json:"version"`
				Append      bool   `json:"append"`
				Rollback    string `json:"rollback"`
				RevisionCAS bool   `json:"revisionCAS"`
				Idempotency bool   `json:"idempotency"`
			} `json:"sessionHistoryMutations"`
			ExplicitZeroTools bool `json:"explicitZeroTools"`
		} `json:"capabilities"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&health); err != nil {
		return result, errors.New("invalid runtime capability response")
	}
	result.Version = health.Version
	tc := health.Capabilities.TurnContext
	result.TurnContext = tc.Field == "appendSystemPrompt" && tc.MaxCodePoints == MaxTurnContextRunes && tc.Persistence == "run-only"
	h := health.Capabilities.History
	result.SessionAppend = h.Version == 1 && h.Append && h.RevisionCAS && h.Idempotency
	result.SessionRollback = h.Version == 1 && h.Rollback == "keep-anchor" && h.RevisionCAS && h.Idempotency
	result.ExplicitZeroTools = health.Capabilities.ExplicitZeroTools
	return result, nil
}

func (c *Client) StreamTurnContextRun(ctx context.Context, baseURL, agentName, apiKey string, req TurnContextRunRequest) (io.ReadCloser, error) {
	body, err := MarshalTurnContextRun(req)
	if err != nil {
		return nil, err
	}
	capabilities, err := c.ProbeMemoryCapabilities(ctx, baseURL, apiKey)
	if err != nil {
		return nil, err
	}
	if !capabilities.TurnContext {
		return nil, ErrTurnContextUnsupported
	}
	return c.StreamRun(ctx, baseURL, agentName, apiKey, body, "")
}
