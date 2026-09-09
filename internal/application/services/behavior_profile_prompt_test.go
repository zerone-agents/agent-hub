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
