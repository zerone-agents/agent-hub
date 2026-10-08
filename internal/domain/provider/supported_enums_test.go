package provider

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// 探测端点枚举白名单的唯一来源在此：handler 校验与错误文案都从这两个函数取值，
// 新增 Protocol/AuthStyle 只需改这里。本测试钉住清单防止意外增删。
func TestSupportedEnums_Stable(t *testing.T) {
	require.Equal(t,
		[]Protocol{ProtocolAnthropic, ProtocolOpenAI, ProtocolMinerU, ProtocolPaddleOCR},
		SupportedProtocols())
	require.Equal(t,
		[]AuthStyle{AuthStyleAPIKey, AuthStyleAuthToken, AuthStyleNoAuth},
		SupportedAuthStyles())
}
