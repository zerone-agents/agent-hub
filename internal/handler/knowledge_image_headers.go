package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// KnowledgeImageHeaders runs before authentication so scoped image and source
// file denials have the same cache and MIME policy as successful reads.
func KnowledgeImageHeaders(c *gin.Context) {
	if c.Request.Method == http.MethodGet {
		const prefix = "/api/v1/admin/knowledge/datasets/"
		path := c.Request.URL.Path
		if strings.HasPrefix(path, prefix) {
			dataset, resource, scoped := strings.Cut(strings.TrimPrefix(path, prefix), "/")
			image := strings.HasPrefix(resource, "images/")
			document, action, source := strings.Cut(strings.TrimPrefix(resource, "documents/"), "/")
			file := strings.HasPrefix(resource, "documents/") && source && document != "" && (action == "preview" || action == "download")
			if scoped && dataset != "" && (image || file) {
				c.Header("Cache-Control", "no-store")
				c.Header("X-Content-Type-Options", "nosniff")
			}
		}
	}
	c.Next()
}
