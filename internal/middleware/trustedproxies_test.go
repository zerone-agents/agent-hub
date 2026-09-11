package middleware

import (
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
