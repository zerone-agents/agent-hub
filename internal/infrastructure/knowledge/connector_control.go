package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	domain "control-panel/internal/domain/knowledge"
)

func (c *RemoteMultiragEngine) resumeConnector(ctx context.Context, connectorID string, body domain.Object) (any, error) {
	resume, ok := body["resume"].(bool)
	if !ok {
		return nil, domain.NewBadRequestError("resume 必须为布尔值")
	}
	path := "/api/v1/connectors/" + url.PathEscape(connectorID)
	var data json.RawMessage
	_, err := c.doJSON(ctx, http.MethodPost, path+"/resume", nil, domain.Object{"resume": resume}, &data)
	if err == nil {
		if !bytes.Equal(bytes.TrimSpace(data), []byte("true")) {
			return nil, domain.NewUpstreamError("连接器操作未确认受理，请刷新状态后核对", nil)
		}
		return true, nil
	}
	// Only a retired route is safe to adapt. A failed or ambiguous write may
	// already have taken effect, so business/transport/server errors never replay.
	var failure *knowledgeUpstreamFailure
	if ctx.Err() != nil || !errors.As(err, &failure) || (failure.httpStatus != http.StatusNotFound && failure.httpStatus != http.StatusMethodNotAllowed) {
		return nil, err
	}
	status := "2"
	if resume {
		status = "5"
	}
	var connector map[string]any
	if _, err := c.doJSON(ctx, http.MethodPatch, path, nil, domain.Object{"status": status}, &connector); err != nil {
		return nil, err
	}
	// Newer services return the saved connector, not the legacy boolean ack.
	// A resumed connector may already be running by the time it is serialized.
	observed, _ := connector["status"].(string)
	if connector["id"] != connectorID || (observed != status && !(resume && observed == "1")) {
		return nil, domain.NewUpstreamError("连接器操作已提交，但状态未确认，请刷新后核对", nil)
	}
	return connector, nil
}
