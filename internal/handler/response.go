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

// respondErrorWithParams extends respondError with interpolation params for
// the frontend en-mode translation (issue #201 B 档：error 中文原文 + code
// 稳定码 + params 插值参数；params 为空时键省略，保持旧载荷形状不变).
func respondErrorWithParams(c *gin.Context, code int, errCode, msg string, params map[string]string) {
	body := gin.H{
		"success": false,
		"error":   msg,
		"code":    errCode,
	}
	if len(params) > 0 {
		body["params"] = params
	}
	c.JSON(code, body)
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
