package handler

import (
	"control-panel/internal/domain/knowledge"
	"control-panel/internal/domain/tenant"
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *KnowledgeHandler) CheckEmbedding(c *gin.Context) {
	var req knowledge.EmbeddingCheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidParameter, "嵌入模型检查参数无效")
		return
	}
	result, err := h.service.CheckEmbedding(c.Request.Context(), tenant.GetTenantID(c), c.Param("datasetId"), req)
	if err != nil {
		respondKnowledgeError(c, err)
		return
	}
	respondSuccess(c, result)
}

func (h *KnowledgeHandler) PreviewDocument(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	stream, err := h.service.PreviewDocument(c.Request.Context(), c.Param("datasetId"), c.Param("documentId"))
	if err != nil {
		respondKnowledgeError(c, err)
		return
	}
	respondStream(c, stream, "application/octet-stream")
}
