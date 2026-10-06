package handler

import (
	"control-panel/internal/domain/knowledge"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
)

func registerKnowledgePageRoutes(write, read *gin.RouterGroup, h *KnowledgeHandler) {
	read.GET("/datasets/tags/aggregation", h.knowledgePage("tags-aggregation", false))
	read.GET("/datasets/metadata/keys", h.knowledgePage("metadata-keys", false))
	read.GET("/datasets/metadata/flattened", h.knowledgePage("metadata-flattened", false))
	write.POST("/datasets/:datasetId/sources/oauth/:provider/start", h.knowledgePage("oauth-start", true))
	write.POST("/datasets/:datasetId/sources/oauth/:provider/result", h.knowledgePage("oauth-result", true))
	read.GET("/datasets/:datasetId/metadata/summary", h.knowledgePage("metadata-summary", false))
	read.GET("/datasets/:datasetId/documents/filters", h.knowledgePage("documents-filter", false))
	write.POST("/datasets/:datasetId/documents/create-empty", h.knowledgePage("create-empty", true))
	write.POST("/datasets/:datasetId/documents/create-web", h.knowledgePage("create-web", true))
	write.POST("/datasets/:datasetId/documents/batch-update-status", h.knowledgePage("document-status", true))
	write.PUT("/datasets/:datasetId/documents/:documentId/metadata/config", h.knowledgePage("document-metadata-config", true))
	write.PATCH("/datasets/:datasetId/documents/metadatas", h.knowledgePage("document-metadatas", true))
	read.GET("/datasets/:datasetId/metadata/config", h.knowledgePage("metadata-config-get", false))
	write.PUT("/datasets/:datasetId/metadata/config", h.knowledgePage("metadata-config-put", true))
	read.GET("/datasets/:datasetId/tags", h.knowledgePage("tags-list", false))
	write.PUT("/datasets/:datasetId/tags", h.knowledgePage("tags-rename", true))
	write.DELETE("/datasets/:datasetId/tags", h.knowledgePage("tags-delete", true))
	read.GET("/datasets/:datasetId/ingestions/summary", h.knowledgePage("ingestions-summary", false))
	read.GET("/datasets/:datasetId/ingestions", h.knowledgePage("ingestions-list", false))
	read.GET("/datasets/:datasetId/ingestions/:logId", h.knowledgePage("ingestions-detail", false))
	read.GET("/datasets/:datasetId/index", h.knowledgePage("index-get", false))
	write.POST("/datasets/:datasetId/index", h.knowledgePage("index-run", false))
	write.DELETE("/datasets/:datasetId/index", h.knowledgePage("index-delete", false))
	write.POST("/datasets/:datasetId/index/cancel", h.knowledgePage("index-cancel", true))
	read.GET("/datasets/:datasetId/graph/search", h.knowledgePage("graph-search", false))
	read.GET("/datasets/:datasetId/graph", h.knowledgePage("graph-get", false))
	// Connector results may contain source credentials. All connector reads use
	// the existing maintainer/admin group, including associated-source listing.
	write.GET("/datasets/:datasetId/connectors", h.knowledgePage("connectors-linked", false))
	write.GET("/datasets/:datasetId/sources", h.knowledgePage("connectors-list", false))
	write.POST("/datasets/:datasetId/sources", h.knowledgePage("connectors-create", true))
	write.GET("/datasets/:datasetId/sources/:connectorId", h.knowledgePage("connector-get", false))
	write.PATCH("/datasets/:datasetId/sources/:connectorId", h.knowledgePage("connector-update", true))
	write.DELETE("/datasets/:datasetId/sources/:connectorId", h.knowledgePage("connector-delete", false))
	write.GET("/datasets/:datasetId/sources/:connectorId/logs", h.knowledgePage("connector-logs", false))
	write.POST("/datasets/:datasetId/sources/:connectorId/resume", h.knowledgePage("connector-resume", true))
	write.POST("/datasets/:datasetId/sources/:connectorId/rebuild", h.knowledgePage("connector-rebuild", false))
	write.PUT("/datasets/:datasetId/connectors/:connectorId", h.knowledgePage("connector-link", true))
	write.DELETE("/datasets/:datasetId/connectors/:connectorId", h.knowledgePage("connector-unlink", false))

}
func (h *KnowledgeHandler) knowledgePage(operation string, body bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		req := knowledge.PageRequest{Operation: operation, OAuthProvider: c.Param("provider"), DatasetID: c.Param("datasetId"), DocumentID: c.Param("documentId"), ConnectorID: c.Param("connectorId"), LogID: c.Param("logId"), Query: c.Request.URL.Query()}
		if body {
			if err := c.ShouldBindJSON(&req.Body); err != nil || req.Body == nil {
				respondError(c, http.StatusBadRequest, ErrCodeInvalidParameter, "请求内容必须为JSON对象")
				return
			}
		}
		result, err := h.service.KnowledgePage(c.Request.Context(), req)
		if err != nil {
			var upstream *knowledge.TagAggregationUpstreamError
			if errors.As(err, &upstream) {
				c.JSON(knowledge.StatusCode(err), gin.H{"success": false, "code": ErrCodeKnowledgeError, "error": upstream.Error(), "data": gin.H{"upstream_code": upstream.UpstreamCode}})
				return
			}
			respondKnowledgeError(c, err)
			return
		}
		respondSuccess(c, result)
	}
}
