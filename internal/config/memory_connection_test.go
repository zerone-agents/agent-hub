package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemoryEnabledByDefaultAndOptionalWithoutBackend(t *testing.T) {
	previous := Get()
	t.Cleanup(func() { SetGlobalConfig(previous) })
	for _, tc := range []struct {
		name, flag, url, key     string
		wantEnabled, wantRuntime bool
	}{
		{name: "default with shared connection", url: "http://multirag:9380", key: "SDK_SECRET", wantEnabled: true, wantRuntime: true},
		{name: "default without optional integration", wantEnabled: true},
		{name: "incomplete shared URL does not block Hub", url: "http://multirag:9380", wantEnabled: true},
		{name: "incomplete shared key does not block Hub", key: "SDK_SECRET", wantEnabled: true},
		{name: "explicit opt out", flag: "false", url: "http://multirag:9380", key: "SDK_SECRET"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MEMORY_ENABLED", tc.flag)
			t.Setenv("MULTIRAG_BASE_URL", tc.url)
			t.Setenv("MULTIRAG_API_KEY", tc.key)
			t.Setenv("MEMORY_MULTIRAG_URL", "")
			t.Setenv("MEMORY_MULTIRAG_KEY", "")
			t.Setenv("MEMORY_CLIENTS_JSON", " [ ] ")
			t.Setenv("MEMORY_MANAGERS_JSON", "[]")
			t.Setenv("MEMORY_WRAPPING_KEY", "")
			cfg, err := LoadConfig()
			require.NoError(t, err)
			require.Equal(t, tc.wantEnabled, cfg.Memory.Enabled)
			require.Equal(t, tc.wantRuntime, cfg.Memory.RuntimeEnabled())
			if !tc.wantRuntime {
				clients, err := cfg.Memory.Clients()
				require.NoError(t, err)
				require.Empty(t, clients)
				managers, err := cfg.Memory.Managers()
				require.NoError(t, err)
				require.Empty(t, managers)
			}
		})
	}
}

func TestMemoryBindingsStillRequireBackendWhenEnabledByDefault(t *testing.T) {
	previous := Get()
	t.Cleanup(func() { SetGlobalConfig(previous) })
	t.Setenv("MEMORY_ENABLED", "")
	t.Setenv("MULTIRAG_BASE_URL", "")
	t.Setenv("MULTIRAG_API_KEY", "")
	t.Setenv("MEMORY_MULTIRAG_URL", "")
	t.Setenv("MEMORY_MULTIRAG_KEY", "")
	t.Setenv("MEMORY_WRAPPING_KEY", "")
	t.Setenv("MEMORY_CLIENTS_JSON", "[]")
	t.Setenv("MEMORY_MANAGERS_JSON", "[]")
	for _, name := range []string{"MEMORY_CLIENTS_JSON", "MEMORY_MANAGERS_JSON"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, `[{"token":"`+strings.Repeat("s", 32)+`"}]`)
			_, err := LoadConfig()
			require.Error(t, err, "configured bindings cannot silently lose their backend")
			require.NotContains(t, err.Error(), strings.Repeat("s", 32))
		})
	}
}

func TestMemoryConnectionResolutionThroughLoadConfig(t *testing.T) {
	previous := Get()
	t.Cleanup(func() { SetGlobalConfig(previous) })
	t.Setenv("MULTIRAG_BASE_URL", "http://shared-multirag:9380")
	t.Setenv("MULTIRAG_API_KEY", "SHARED_SDK_SECRET")
	t.Setenv("MEMORY_CLIENTS_JSON", "[]")
	t.Setenv("MEMORY_MANAGERS_JSON", "[]")
	t.Setenv("MEMORY_WRAPPING_KEY", "")
	for _, tc := range []struct {
		name, url, key, expectedURL, expectedKey string
		enabled, wantError                       bool
	}{
		{name: "shared defaults", enabled: true, expectedURL: "http://shared-multirag:9380", expectedKey: "SHARED_SDK_SECRET"},
		{name: "isolated override", enabled: true, url: "https://isolated.example", key: "ISOLATED_SDK_SECRET", expectedURL: "https://isolated.example", expectedKey: "ISOLATED_SDK_SECRET"},
		{name: "URL alone cannot receive shared credential", enabled: true, url: "https://different.example", wantError: true},
		{name: "key alone cannot select wrong account", enabled: true, key: "ISOLATED_SDK_SECRET", wantError: true},
		{name: "disabled ignores dormant override", url: "invalid", expectedURL: "invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.enabled {
				t.Setenv("MEMORY_ENABLED", "true")
			} else {
				t.Setenv("MEMORY_ENABLED", "false")
			}
			t.Setenv("MEMORY_MULTIRAG_URL", tc.url)
			t.Setenv("MEMORY_MULTIRAG_KEY", tc.key)
			cfg, err := LoadConfig()
			if tc.wantError {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "SHARED_SDK_SECRET")
				require.NotContains(t, err.Error(), "ISOLATED_SDK_SECRET")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedURL, cfg.Memory.MultiragURL)
			require.Equal(t, tc.expectedKey, cfg.Memory.MultiragKey)
			require.Empty(t, cfg.Memory.WrappingKey, "DB provisioning happens after config parsing")
		})
	}
	t.Setenv("MEMORY_ENABLED", "true")
	t.Setenv("MEMORY_MULTIRAG_URL", "")
	t.Setenv("MEMORY_MULTIRAG_KEY", "")
	t.Setenv("MULTIRAG_API_KEY", "")
	cfg, err := LoadConfig()
	require.NoError(t, err, "an unconfigured optional backend must not block Hub startup")
	require.False(t, cfg.Memory.RuntimeEnabled())
}
