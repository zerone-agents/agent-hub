package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestApplyTrustedProxies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	require.NoError(t, ApplyTrustedProxies(r, nil))                          // 空 = 不信任任何代理
	require.NoError(t, ApplyTrustedProxies(r, []string{"10.0.0.0/8"}))       // 合法 CIDR
	require.Error(t, ApplyTrustedProxies(gin.New(), []string{"not-a-cidr"})) // 非法 → 错误（fatal 驱动）
}

// RemoteIP 信任边界 e2e（spec §5.2/§8；终审 Minor#2 补测）：
// 未配置可信代理 → 伪造 X-Forwarded-For 不生效，ClientIP 取对端地址；
// 对端落入可信 CIDR → 转发头生效。
// httptest.NewRequest 默认 RemoteAddr = 192.0.2.1:1234。
func TestApplyTrustedProxiesClientIPBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	do := func(trusted []string, xff string) string {
		r := gin.New()
		require.NoError(t, ApplyTrustedProxies(r, trusted))
		var got string
		r.GET("/ip", func(c *gin.Context) { got = c.ClientIP() })
		req := httptest.NewRequest(http.MethodGet, "/ip", nil)
		if xff != "" {
			req.Header.Set("X-Forwarded-For", xff)
		}
		r.ServeHTTP(httptest.NewRecorder(), req)
		return got
	}

	// 未信任任何代理：伪造头被忽略，取对端地址（审计 IP 不可被客户端污染）
	require.Equal(t, "192.0.2.1", do(nil, "1.2.3.4"))
	// 对端在可信 CIDR 内（反代部署形态）：转发头生效
	require.Equal(t, "1.2.3.4", do([]string{"192.0.2.0/24"}, "1.2.3.4"))
	// 对端不在可信 CIDR 内：伪造头同样被忽略（httptest 默认对端 192.0.2.1）
	require.Equal(t, "192.0.2.1", do([]string{"10.0.0.0/8"}, "1.2.3.4"))
}
