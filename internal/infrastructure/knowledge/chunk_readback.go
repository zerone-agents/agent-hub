package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const chunkVisibilityBudget = time.Second
const chunkVisibilityRetryDelay = 100 * time.Millisecond

// Only the post-PATCH GET is replayed. The deadline covers network time as well
// as waits, and a caller's shorter deadline/cancellation takes precedence.
func (c *RemoteMultiragEngine) readVisibleChunk(ctx context.Context, path string) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, chunkVisibilityBudget)
	defer cancel()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		raw, retryable, err := c.readChunkVisibilityAttempt(ctx, path)
		if err == nil || !retryable {
			return raw, err
		}
		timer := time.NewTimer(chunkVisibilityRetryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// 102 also represents ownership/validation failures upstream. Require the
// pinned single-chunk GET's exact not-found message, never just its code.
func (c *RemoteMultiragEngine) readChunkVisibilityAttempt(ctx context.Context, path string) (map[string]any, bool, error) {
	req, err := c.newRequest(ctx, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, false, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, false, fmt.Errorf("chunk readback HTTP %d", resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&env); err != nil {
		return nil, false, err
	}
	code, present := env.statusCode()
	if !present || (env.Code != nil && env.RetCode != nil && *env.Code != *env.RetCode) {
		return nil, false, fmt.Errorf("chunk readback missing or conflicting business status")
	}
	if code != 0 {
		message := strings.TrimSpace(env.message())
		missing := message == "Chunk not found!" || message == "Chunk not found"
		coherentMessage := env.RetMsg == "" || env.Message == "" || strings.TrimSpace(env.RetMsg) == strings.TrimSpace(env.Message)
		return nil, resp.StatusCode == http.StatusOK && code == 102 && missing && coherentMessage && env.Error == "", fmt.Errorf("chunk readback business code %d", code)
	}
	var raw map[string]any
	if len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, &raw); err != nil {
			return nil, false, err
		}
	}
	return raw, false, nil
}
