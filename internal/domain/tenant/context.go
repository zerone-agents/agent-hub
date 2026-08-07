package tenant

import (
	"github.com/gin-gonic/gin"
)

func GetTenantID(c *gin.Context) string {
	if tenantID, exists := c.Get("tenant_id"); exists {
		return tenantID.(string)
	}
	return ""
}

func SetTenantID(c *gin.Context, tenantID string) {
	c.Set("tenant_id", tenantID)
}
