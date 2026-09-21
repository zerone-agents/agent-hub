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

// respondError writes the error envelope with both the human-readable message
// (kept verbatim for existing clients & tests) and a stable machine-readable
// code (issue #149 i18n 方案 B 双写：P6 前端按 code 翻译，error 过渡期保留).
func respondError(c *gin.Context, code int, errCode, msg string) {
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
