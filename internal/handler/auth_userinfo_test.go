package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// UserInfo 必须返回中间件注入的真实 tenant_id（authority 字段），
// 而不是历史占位空串。
func TestUserInfoReturnsRealTenantID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/auth/userinfo", nil)
	c.Set("user_id", "42")
	c.Set("user_name", "alice")
	c.Set("roles", []string{"admin"})
	tenant.SetTenantID(c, "tenant-acme")

	UserInfo(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			TenantID string `json:"tenant_id"`
			OrgID    any    `json:"org_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Equal(t, "tenant-acme", resp.Data.TenantID, "tenant_id must reflect the tenant injected by the middleware")
}

// builtin 模式（tenant_id = "default"）下 tenant_id 同样返回真值。
func TestUserInfoBuiltinTenantIDDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/auth/userinfo", nil)
	c.Set("user_id", "1")
	tenant.SetTenantID(c, tenant.DefaultID)

	UserInfo(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data struct {
			TenantID string `json:"tenant_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, tenant.DefaultID, resp.Data.TenantID)
}
