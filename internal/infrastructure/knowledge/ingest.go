package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	domain "control-panel/internal/domain/knowledge"
)

func (c *RemoteMultiragEngine) IngestDocuments(ctx context.Context, req domain.DocumentIngestRequest) error {
	if err := req.Validate(); err != nil {
		return err
	}
	var acknowledgement json.RawMessage
	if _, err := c.doJSON(ctx, http.MethodPost, "/api/v1/documents/ingest", nil, req, &acknowledgement); err != nil {
		return err
	}
	if !bytes.Equal(bytes.TrimSpace(acknowledgement), []byte("true")) {
		return domain.NewUpstreamError("文档摄取请求未确认受理，请刷新状态后重试", nil)
	}
	return nil
}
