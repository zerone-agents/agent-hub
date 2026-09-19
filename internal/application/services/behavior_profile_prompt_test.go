package services

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestComposePersonalitySystemPrompt(t *testing.T) {
	t.Run("prompt is primary and boundary is explicit", func(t *testing.T) {
		got := composePersonalitySystemPrompt("你是财务总管。", "你谨慎，但在证据充分时会越级。")
		require.Contains(t, got, "你是财务总管。")
		require.Contains(t, got, "[人格原稿]")
		require.Contains(t, got, "你谨慎，但在证据充分时会越级。")
		require.Contains(t, got, "不授予工具、数据、通信、组织层级或越级权限")
		require.Contains(t, got, "人格判断只来自这份原稿")
	})

	t.Run("base prompt is unchanged without personality", func(t *testing.T) {
		const base = "你是财务总管。"
		require.Equal(t, base, composePersonalitySystemPrompt(base, ""))
	})
}
