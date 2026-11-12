package services

import (
	"testing"

	"control-panel/internal/domain/agent"

	"github.com/stretchr/testify/require"
)

// TestValidateConfig_Thinking 锁定 thinking 形态校验契约（issue #257）：
// 显式 null = 清除合法；对象时 type 命中 SDK 枚举、budgetTokens 为
// 1..128000 的整数、带 budgetTokens 必须带 type（半态拦截——无 type 的
// 对象会炸 runtime Zod 必填与 deployer fail-fast）。
func TestValidateConfig_Thinking(t *testing.T) {
	t.Run("null 清除合法", func(t *testing.T) {
		require.NoError(t, ValidateConfig(map[string]interface{}{
			"systemPrompt": "x", "thinking": nil,
		}))
	})

	t.Run("absent 不校验", func(t *testing.T) {
		require.NoError(t, ValidateConfig(map[string]interface{}{
			"systemPrompt": "x",
		}))
	})

	t.Run("非对象拒绝", func(t *testing.T) {
		err := ValidateConfig(map[string]interface{}{
			"systemPrompt": "x", "thinking": "disabled",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "thinking 必须是对象或 null")
	})

	t.Run("三枚举值放行", func(t *testing.T) {
		for _, typ := range []string{"adaptive", "enabled", "disabled"} {
			require.NoError(t, ValidateConfig(map[string]interface{}{
				"systemPrompt": "x", "thinking": map[string]interface{}{"type": typ},
			}), "type %s must pass", typ)
		}
	})

	t.Run("枚举外拒绝", func(t *testing.T) {
		err := ValidateConfig(map[string]interface{}{
			"systemPrompt": "x", "thinking": map[string]interface{}{"type": "on"},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "无效的 thinking.type: on")
	})

	t.Run("budgetTokens 合法范围放行", func(t *testing.T) {
		require.NoError(t, ValidateConfig(map[string]interface{}{
			"systemPrompt": "x",
			"thinking":     map[string]interface{}{"type": "enabled", "budgetTokens": float64(4096)},
		}))
	})

	t.Run("budgetTokens 越界/非整数拒绝", func(t *testing.T) {
		for _, bt := range []interface{}{float64(0), float64(128001), 1.5, "4096"} {
			err := ValidateConfig(map[string]interface{}{
				"systemPrompt": "x",
				"thinking":     map[string]interface{}{"type": "enabled", "budgetTokens": bt},
			})
			require.Error(t, err, "budgetTokens %#v must be rejected", bt)
			require.Contains(t, err.Error(), "thinking.budgetTokens 必须是 1 到 128000 的整数")
		}
	})

	t.Run("半态：budgetTokens 无 type 拒绝", func(t *testing.T) {
		err := ValidateConfig(map[string]interface{}{
			"systemPrompt": "x",
			"thinking":     map[string]interface{}{"budgetTokens": float64(4096)},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "设置 thinking.budgetTokens 必须同时提供 thinking.type")
	})
}

// TestThinkingConfigKeys 锁定 thinking 的 config map 契约（issue #257）：
// 对象解包（type trim）、显式 null 清除、absent 不变更；pack 未设置 → nil、
// 已设置 → 嵌套对象（对齐 SDK/deployer 契约形态）。
func TestThinkingConfigKeys(t *testing.T) {
	t.Run("unpack 对象", func(t *testing.T) {
		cfg := &agent.AgentConfig{}
		require.NoError(t, unpackConfigToModel(map[string]interface{}{
			"systemPrompt": "p",
			"thinking":     map[string]interface{}{"type": " disabled ", "budgetTokens": float64(8192)},
		}, cfg, ""))
		require.Equal(t, "disabled", cfg.ThinkingType, "type must be trimmed")
		require.NotNil(t, cfg.ThinkingBudgetTokens)
		require.Equal(t, 8192, *cfg.ThinkingBudgetTokens)
	})

	t.Run("unpack null 清除", func(t *testing.T) {
		cfg := &agent.AgentConfig{ThinkingType: "disabled", ThinkingBudgetTokens: intPtr(4096)}
		require.NoError(t, unpackConfigToModel(map[string]interface{}{
			"systemPrompt": "p", "thinking": nil,
		}, cfg, ""))
		require.Empty(t, cfg.ThinkingType)
		require.Nil(t, cfg.ThinkingBudgetTokens)
	})

	t.Run("unpack absent 不变更", func(t *testing.T) {
		cfg := &agent.AgentConfig{ThinkingType: "disabled"}
		require.NoError(t, unpackConfigToModel(map[string]interface{}{
			"systemPrompt": "p",
		}, cfg, ""))
		require.Equal(t, "disabled", cfg.ThinkingType)
	})

	t.Run("pack 未设置为 nil", func(t *testing.T) {
		m := modelToConfigMap(&agent.AgentConfig{}, "")
		v, ok := m["thinking"]
		require.True(t, ok, "thinking key must always be present")
		require.Nil(t, v)
	})

	t.Run("pack 已设置为嵌套对象", func(t *testing.T) {
		m := modelToConfigMap(&agent.AgentConfig{ThinkingType: "disabled", ThinkingBudgetTokens: intPtr(4096)}, "")
		th, ok := m["thinking"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, "disabled", th["type"])
		require.Equal(t, float64(4096), th["budgetTokens"])

		// 无 budgetTokens 时不携带该键
		m2 := modelToConfigMap(&agent.AgentConfig{ThinkingType: "disabled"}, "")
		th2, ok := m2["thinking"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, map[string]interface{}{"type": "disabled"}, th2)
	})
}
