package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// KnowledgeImageHeaders runs before authentication so image denials have the
// same cache and MIME policy as successful reads. Other endpoints are untouched.
func KnowledgeImageHeaders(c *gin.Context) {
	if c.Request.Method == http.MethodGet {
		const prefix = "/api/v1/admin/knowledge/datasets/"
		path := c.Request.URL.Path
		if strings.HasPrefix(path, prefix) {
			dataset, _, image := strings.Cut(strings.TrimPrefix(path, prefix), "/images/")
			if image && dataset != "" && !strings.Contains(dataset, "/") {
				c.Header("Cache-Control", "no-store")
				c.Header("X-Content-Type-Options", "nosniff")
			}
		}
	}
	c.Next()
}
