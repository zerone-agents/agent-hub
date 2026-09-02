package services

import (
	"strings"
	"testing"

	"control-panel/internal/domain/agent"
)

// TestMaxSessionQueriesConfigKeys locks the issue #111 rename: the config map
// speaks maxSessionQueries only, and legacy maxSessionTurns requests are
// rejected at the unpack entry before any field is applied (no partial
// unpacking).
func TestMaxSessionQueriesConfigKeys(t *testing.T) {
	t.Run("unpack 新 key", func(t *testing.T) {
		cfg := &agent.AgentConfig{}
		if err := unpackConfigToModel(map[string]interface{}{
			"maxSessionQueries": float64(50),
			"systemPrompt":      "p",
		}, cfg, ""); err != nil {
			t.Fatalf("unpackConfigToModel: %v", err)
		}
		if cfg.MaxSessionQueries == nil || *cfg.MaxSessionQueries != 50 {
			t.Fatalf("cfg.MaxSessionQueries = %v, want *50", cfg.MaxSessionQueries)
		}
	})

	t.Run("unpack 旧 key 拒绝", func(t *testing.T) {
		cfg := &agent.AgentConfig{}
		err := unpackConfigToModel(map[string]interface{}{
			"maxSessionTurns": float64(50),
			"systemPrompt":    "p",
		}, cfg, "")
		if err == nil {
			t.Fatal("unpackConfigToModel must reject legacy maxSessionTurns key")
		}
		if !strings.Contains(err.Error(), "maxSessionTurns") {
			t.Fatalf("error should mention maxSessionTurns, got: %v", err)
		}
		if cfg.MaxSessionQueries != nil {
			t.Fatalf("cfg.MaxSessionQueries must stay nil (no partial unpack), got %v", *cfg.MaxSessionQueries)
		}
		if cfg.SystemPrompt != "" {
			t.Fatalf("cfg.SystemPrompt must stay empty (no partial unpack), got %q", cfg.SystemPrompt)
		}
	})

	t.Run("pack 新 key", func(t *testing.T) {
		cfg := &agent.AgentConfig{MaxSessionQueries: intPtr(50)}
		m := modelToConfigMap(cfg, "")
		if v, ok := m["maxSessionQueries"].(float64); !ok || v != 50 {
			t.Fatalf(`m["maxSessionQueries"] = %#v, want float64(50)`, m["maxSessionQueries"])
		}
		if _, exists := m["maxSessionTurns"]; exists {
			t.Fatal(`packed map must not contain legacy "maxSessionTurns" key`)
		}
	})

	t.Run("pack nil", func(t *testing.T) {
		cfg := &agent.AgentConfig{}
		m := modelToConfigMap(cfg, "")
		v, exists := m["maxSessionQueries"]
		if !exists {
			t.Fatal(`m["maxSessionQueries"] key must be present with explicit nil`)
		}
		if v != nil {
			t.Fatalf(`m["maxSessionQueries"] = %#v, want nil`, v)
		}
	})
}
