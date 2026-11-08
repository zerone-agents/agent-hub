package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/domain/knowledge"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestKnowledgeWriteRecoveryWireStaysFailure(t *testing.T) {
	for _, tc := range []struct {
		code string
		err  error
	}{
		{"knowledge_upload_partial", &knowledge.UploadPartialError{Uploaded: []knowledge.Document{{"id": "d1", "name": "one.txt", "dataset_id": "kb", "run": "0"}}}},
		{"knowledge_chunk_image_unconfirmed", &knowledge.ChunkImageWriteError{Recovery: knowledge.ChunkImageRecovery{Outcome: "partial", Stage: "image_write", Index: "acknowledged", Image: "unchanged", ImageID: "kb-chunk"}}},
	} {
		t.Run(tc.code, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			respondKnowledgeError(ctx, tc.err)
			require.Equal(t, http.StatusBadGateway, recorder.Code)
			var body struct {
				Success bool           `json:"success"`
				Code    string         `json:"code"`
				Data    map[string]any `json:"data"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
			require.False(t, body.Success)
			require.Equal(t, tc.code, body.Code)
			require.Equal(t, false, body.Data["retry_safe"])
			require.Equal(t, "partial", body.Data["outcome"])
		})
	}
}
