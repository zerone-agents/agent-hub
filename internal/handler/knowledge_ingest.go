package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"control-panel/internal/domain/knowledge"
	"github.com/gin-gonic/gin"
)

func (h *KnowledgeHandler) IngestDocuments(c *gin.Context) {
	var req knowledge.DocumentIngestRequest
	decoder := json.NewDecoder(c.Request.Body)
	if err := decoder.Decode(&req); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidParameter, err.Error())
		return
	}
	if decoder.Decode(new(any)) != io.EOF {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidParameter, "请求体必须是单个JSON对象")
		return
	}
	if err := h.service.IngestDocuments(c.Request.Context(), req); err != nil {
		respondKnowledgeError(c, err)
		return
	}
	// Receipt only: callers must poll document run/progress/chunk_count and
	// retrieve chunks independently before reporting worker completion.
	respondSuccess(c, true)
}
