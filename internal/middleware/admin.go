package middleware

import (
	"log"
	"net/http"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
)

// RequireAdmin returns middleware that verifies the user has the "agents-admin" role.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		roles, exists := c.Get("roles")
		if !exists {
			respondForbidden(c, "admin permission required")
			return
		}

		if !isAdminRole(roles) {
			userID, _ := c.Get("user_id")
			userName, _ := c.Get("user_name")

			log.Printf("[WARN] non-admin user attempted admin action | user_id=%v user_name=%v path=%s method=%s time=%s",
				userID,
				userName,
				c.Request.URL.Path,
				c.Request.Method,
				time.Now().Format(time.RFC3339),
			)

			respondForbidden(c, "admin permission required")
			return
		}
		c.Next()
	}
}

func isAdminRole(roles interface{}) bool {
	roleList, ok := roles.([]*casdoorsdk.Role)
	if !ok {
		return false
	}

	for _, role := range roleList {
		if role != nil && role.Name == "agents-admin" {
			return true
		}
	}
	return false
}

func respondForbidden(c *gin.Context, msg string) {
	c.JSON(http.StatusForbidden, gin.H{
		"success": false,
		"error":   msg,
	})
	c.Abort()
}
