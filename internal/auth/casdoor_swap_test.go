package auth

// 评审实证的缓存泄漏：SwapCasdoorForTest 此前只换 client/casdoorConfig，
// 第三处全局态 clientsByOrg（按组织 client 缓存）没有清理点——登录审计测试
// 的成功路径经 SyncMembership → defaultFetchUser → ClientForOrg 把假 endpoint
// 的 client 写进缓存，restore 后残留，后续调用拿到已关闭 endpoint 的 client
// （connection refused）。本测试钉住「swap 清空、restore 还原」契约。

import (
	"testing"

	"control-panel/internal/config"

	"github.com/stretchr/testify/require"
)

func TestSwapCasdoorForTestRestoresOrgClientCache(t *testing.T) {
	restoreA := SwapCasdoorForTest(&config.CasdoorConfig{Endpoint: "http://a.example", ClientID: "a"})
	t.Cleanup(restoreA)
	cachedA := ClientForOrg("swap-test-org")
	require.Equal(t, "http://a.example", cachedA.Endpoint, "A 配置下缓存按 A 端点构建")

	restoreB := SwapCasdoorForTest(&config.CasdoorConfig{Endpoint: "http://b.example", ClientID: "b"})
	require.Equal(t, "http://b.example", ClientForOrg("swap-test-org").Endpoint,
		"swap 后缓存必须清空，按新配置重建（修复前命中残留的 A client）")
	restoreB()

	require.Equal(t, "http://a.example", ClientForOrg("swap-test-org").Endpoint,
		"restore 后旧缓存还原（测试隔离）")
}
