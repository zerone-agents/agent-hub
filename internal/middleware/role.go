package middleware

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// RequireRole allows the request when the user's normalized roles (set by the
// auth middleware as []string) intersect the given allowed set. Missing or
// mistyped roles are treated as no match (forbidden).
func RequireRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	return func(c *gin.Context) {
		val, exists := c.Get("roles")
		if !exists {
			respondForbidden(c, "权限不足")
			return
		}
		userRoles, ok := val.([]string)
		if !ok {
			respondForbidden(c, "权限不足")
			return
		}
		for _, r := range userRoles {
			if allowed[r] {
				c.Next()
				return
			}
		}
		userID, _ := c.Get("user_id")
		log.Printf("[WARN] forbidden action | user_id=%v path=%s method=%s roles=%v time=%s",
			userID, c.Request.URL.Path, c.Request.Method, userRoles, time.Now().Format(time.RFC3339))
		respondForbidden(c, "权限不足")
	}
}

// RequireAdmin guards user-management routes (admin only).
func RequireAdmin() gin.HandlerFunc { return RequireRole("admin") }

// RequireManager guards business-resource admin routes (admin or maintainer).
func RequireManager() gin.HandlerFunc { return RequireRole("admin", "maintainer") }

func respondForbidden(c *gin.Context, msg string) {
	c.JSON(http.StatusForbidden, gin.H{"success": false, "error": msg})
	c.Abort()
}
