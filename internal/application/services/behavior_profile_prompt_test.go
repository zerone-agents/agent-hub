package services

import (
	"strings"
	"testing"

	"control-panel/internal/domain/agent"

	"github.com/stretchr/testify/require"
)

func TestComposeBehaviorProfileSystemPrompt(t *testing.T) {
	t.Run("legacy agent stays byte-for-byte unchanged", func(t *testing.T) {
		const base = "你是财务总管。\n"
		require.Equal(t, base, composeBehaviorProfileSystemPrompt(base, nil))
	})

	t.Run("structured profile is projected after authored identity", func(t *testing.T) {
		profile := agent.DefaultBehaviorProfile()
		got := composeBehaviorProfileSystemPrompt("你是财务总管。", &profile)

		require.True(t, strings.HasPrefix(got, "你是财务总管。\n\n"))
		require.Contains(t, got, "[结构化行为人格｜v1]")
		require.Contains(t, got, "层级服从：75")
		require.Contains(t, got, "越级阈值：75")
		require.Contains(t, got, "不授予任何工具、数据或组织权限")
	})
}

func TestComposePersonalitySystemPrompt(t *testing.T) {
	t.Run("prompt is primary and boundary is explicit", func(t *testing.T) {
		got := composePersonalitySystemPrompt("你是财务总管。", "你谨慎，但在证据充分时会越级。", nil)
		require.Contains(t, got, "你是财务总管。")
		require.Contains(t, got, "[人格原稿]")
		require.Contains(t, got, "你谨慎，但在证据充分时会越级。")
		require.Contains(t, got, "不授予工具、数据、通信、组织层级或越级权限")
		require.Contains(t, got, "与原稿冲突，以人格原稿为准")
	})

	t.Run("structured projection remains compatible", func(t *testing.T) {
		profile := agent.DefaultBehaviorProfile()
		got := composePersonalitySystemPrompt("角色", "人格", &profile)
		require.Contains(t, got, "[人格原稿]")
		require.Contains(t, got, "[结构化行为人格｜v1]")
		require.Less(t, strings.Index(got, "[人格原稿]"), strings.Index(got, "[结构化行为人格｜v1]"))
	})
}
