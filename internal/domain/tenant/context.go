package tenant

import (
	"github.com/gin-gonic/gin"
)

// DefaultID is the implicit tenant for builtin (single-tenant) mode and for
// rows predating multi-tenancy.
const DefaultID = "default"

func GetTenantID(c *gin.Context) string {
	if tenantID, exists := c.Get("tenant_id"); exists {
		return tenantID.(string)
	}
	return ""
}

func SetTenantID(c *gin.Context, tenantID string) {
	c.Set("tenant_id", tenantID)
}
