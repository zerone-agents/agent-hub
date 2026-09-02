package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func respondSuccess(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
	})
}

func respondCreated(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    data,
	})
}

func respondError(c *gin.Context, code int, msg string) {
	c.JSON(code, gin.H{
		"success": false,
		"error":   msg,
	})
}

// respondErrorCode writes the error envelope with a stable machine-readable
// code (issue #94 attachment contract). Old clients ignore the extra field.
func respondErrorCode(c *gin.Context, code int, errCode, msg string) {
	c.JSON(code, gin.H{
		"success": false,
		"error":   msg,
		"code":    errCode,
	})
}

func respondMessage(c *gin.Context, code int, msg string) {
	c.JSON(code, gin.H{
		"success": true,
		"message": msg,
	})
}

func stringOrEmpty(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
