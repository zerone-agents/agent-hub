package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func memoryManagerTestConfiguration() MemoryConfig {
	return MemoryConfig{Enabled: true, WrappingKey: strings.Repeat("ab", 32), MultiragURL: "http://127.0.0.1:8123", MultiragKey: "PRIVATE_BACKEND_CREDENTIAL"}
}

func memoryManagerTestValue() MemoryManager {
	return MemoryManager{Token: "PRIVATE_MANAGER_" + strings.Repeat("m", 32), TenantID: "business-tenant", ChatModel: "qwen-plus@dedicated", EmbeddingModel: "337"}
}

func memoryConfigurationJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return string(encoded)
}

func TestMemoryManagerRegistryOnlyConfigurationAndDefaults(t *testing.T) {
	base := memoryManagerTestConfiguration()
	for _, clientsJSON := range []string{"", " \t\n ", "[]"} {
		cfg := base
		cfg.ClientsJSON = clientsJSON
		clients, err := cfg.Clients()
		require.NoError(t, err)
		require.Empty(t, clients)
		for _, managersJSON := range []string{"", " \t\n ", "[]"} {
			cfg.ManagersJSON = managersJSON
			managers, managerErr := cfg.Managers()
			require.NoError(t, managerErr)
			require.Empty(t, managers)
		}
		cfg.ManagersJSON = memoryConfigurationJSON(t, []MemoryManager{memoryManagerTestValue()})
		managers, err := cfg.Managers()
		require.NoError(t, err)
		require.Len(t, managers, 1)
		require.Equal(t, "business-tenant", managers[0].HubTenantID)
		require.Equal(t, 32, managers[0].MaxApplications)
		require.Equal(t, memoryManagerTestValue().Token, managers[0].Token)
		require.Equal(t, "qwen-plus@dedicated", managers[0].ChatModel)
		require.Equal(t, "337", managers[0].EmbeddingModel)
	}
	// Disabled configuration does not parse or validate dormant credentials.
	disabled := MemoryConfig{ClientsJSON: "malformed-static", ManagersJSON: "malformed-manager"}
	managers, err := disabled.Managers()
	require.NoError(t, err)
	require.Empty(t, managers)
}

func TestMemoryManagerConfigurationBoundsAndDelegation(t *testing.T) {
	for _, maximum := range []int{1, 1000} {
		cfg := memoryManagerTestConfiguration()
		manager := memoryManagerTestValue()
		manager.MaxApplications = maximum
		manager.HubTenantID = "delegated-hub"
		manager.ChatModel = strings.Repeat("模", 128)
		cfg.ManagersJSON = memoryConfigurationJSON(t, []MemoryManager{manager})
		values, err := cfg.Managers()
		require.NoError(t, err)
		require.Equal(t, []MemoryManager{manager}, values)
	}
}

func TestMemoryManagerConfigurationStrictValidationAndCredentialIsolation(t *testing.T) {
	base := memoryManagerTestConfiguration()
	manager := memoryManagerTestValue()
	validJSON := memoryConfigurationJSON(t, []MemoryManager{manager})
	wire := func(changes map[string]any) string {
		value := map[string]any{"token": manager.Token, "tenantId": manager.TenantID, "chatModel": manager.ChatModel, "embeddingModel": manager.EmbeddingModel}
		for key, changed := range changes {
			value[key] = changed
		}
		return memoryConfigurationJSON(t, []map[string]any{value})
	}
	for _, tc := range []struct {
		name, raw string
	}{
		{"null array", "null"},
		{"object instead of array", "{}"},
		{"malformed array", "["},
		{"trailing JSON", validJSON + "{}"},
		{"unknown field", wire(map[string]any{"PRIVATE_UNKNOWN_PROPERTY": true})},
		{"short token", wire(map[string]any{"token": "short"})},
		{"token whitespace", wire(map[string]any{"token": strings.Repeat(" ", 32)})},
		{"token newline", wire(map[string]any{"token": manager.Token + "\n"})},
		{"token too long", wire(map[string]any{"token": strings.Repeat("x", 513)})},
		{"empty tenant", wire(map[string]any{"tenantId": ""})},
		{"invalid tenant", wire(map[string]any{"tenantId": "foreign/tenant"})},
		{"wide tenant", wire(map[string]any{"tenantId": strings.Repeat("t", 65)})},
		{"invalid hub tenant", wire(map[string]any{"hubTenantId": "foreign/hub"})},
		{"wide hub tenant", wire(map[string]any{"hubTenantId": strings.Repeat("h", 65)})},
		{"empty chat model", wire(map[string]any{"chatModel": ""})},
		{"empty embedding model", wire(map[string]any{"embeddingModel": ""})},
		{"padded model", wire(map[string]any{"chatModel": " qwen-plus"})},
		{"wide model", wire(map[string]any{"embeddingModel": strings.Repeat("模", 129)})},
		{"model ASCII control", wire(map[string]any{"chatModel": "model\nsecond-line"})},
		{"model Unicode control", wire(map[string]any{"embeddingModel": "model\u0085suffix"})},
		{"model DEL control", wire(map[string]any{"chatModel": "model\x7f"})},
		{"model invalid UTF8", strings.Replace(validJSON, "qwen-plus", string([]byte{0xff}), 1)},
		{"quota zero", wire(map[string]any{"maxApplications": 0})},
		{"quota negative", wire(map[string]any{"maxApplications": -1})},
		{"quota too large", wire(map[string]any{"maxApplications": 1001})},
		{"quota null", wire(map[string]any{"maxApplications": nil})},
		{"quota boolean", wire(map[string]any{"maxApplications": true})},
		{"quota string", wire(map[string]any{"maxApplications": "32"})},
		{"quota fractional", wire(map[string]any{"maxApplications": 1.5})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			cfg.ManagersJSON = tc.raw
			_, err := cfg.Managers()
			require.Error(t, err)
			for _, private := range []string{manager.Token, "PRIVATE_BACKEND_CREDENTIAL", "PRIVATE_UNKNOWN_PROPERTY", "foreign/tenant", "model\nsecond-line"} {
				require.NotContains(t, err.Error(), private)
			}
		})
	}
	for _, tc := range []struct {
		name   string
		second MemoryManager
	}{
		{"duplicate tenant", MemoryManager{Token: strings.Repeat("b", 32), TenantID: manager.TenantID, ChatModel: "356", EmbeddingModel: "337"}},
		{"duplicate manager token", MemoryManager{Token: manager.Token, TenantID: "different-tenant", ChatModel: "356", EmbeddingModel: "337"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			cfg.ManagersJSON = memoryConfigurationJSON(t, []MemoryManager{manager, tc.second})
			_, err := cfg.Managers()
			require.Error(t, err)
			require.NotContains(t, err.Error(), manager.Token)
		})
	}
	base.ManagersJSON = validJSON
	base.ClientsJSON = memoryConfigurationJSON(t, []MemoryClient{{Token: manager.Token, TenantID: "static-tenant", ApplicationID: "static-app", DatasetID: "static-dataset", PolicyVersion: "synthetic-v1"}})
	_, err := base.Managers()
	require.Error(t, err, "ordinary application credentials must never gain tenant-management authority")
	require.NotContains(t, err.Error(), manager.Token)
}

func TestMemoryRegistryOnlyConfigurationPreservesBackendPrerequisitesAndStaticLimit(t *testing.T) {
	for _, mutate := range []func(*MemoryConfig){
		func(c *MemoryConfig) { c.WrappingKey = "invalid" },
		func(c *MemoryConfig) { c.MultiragURL = "" },
		func(c *MemoryConfig) { c.MultiragURL = "https://PRIVATE_URL_USER@example.invalid" },
		func(c *MemoryConfig) { c.MultiragKey = "" },
	} {
		cfg := memoryManagerTestConfiguration()
		cfg.ClientsJSON, cfg.ManagersJSON = "[]", "[]"
		mutate(&cfg)
		_, err := cfg.Clients()
		require.Error(t, err)
		_, err = cfg.Managers()
		require.Error(t, err)
		require.NotContains(t, err.Error(), "PRIVATE_URL_USER")
	}
	cfg := memoryManagerTestConfiguration()
	for _, raw := range []string{"null", "{}", "[]{}"} {
		cfg.ClientsJSON = raw
		_, err := cfg.Clients()
		require.Error(t, err)
	}
	clients := make([]MemoryClient, 0, 33)
	for i := 0; i < 33; i++ {
		clients = append(clients, MemoryClient{Token: fmt.Sprintf("%s%02d", strings.Repeat("s", 32), i), TenantID: "static-tenant", ApplicationID: fmt.Sprintf("app-%02d", i), DatasetID: fmt.Sprintf("dataset-%02d", i), PolicyVersion: "synthetic-v1"})
	}
	cfg.ClientsJSON = memoryConfigurationJSON(t, clients[:32])
	values, err := cfg.Clients()
	require.NoError(t, err)
	require.Len(t, values, 32)
	cfg.ClientsJSON = memoryConfigurationJSON(t, clients)
	_, err = cfg.Clients()
	require.Error(t, err)
}

func TestLoadConfigMemoryManagersEnvironmentAndValidation(t *testing.T) {
	previous := Get()
	t.Cleanup(func() { SetGlobalConfig(previous) })
	manager := memoryManagerTestValue()
	managerJSON := memoryConfigurationJSON(t, []MemoryManager{manager})
	t.Setenv("MEMORY_ENABLED", "true")
	t.Setenv("MEMORY_CLIENTS_JSON", "[]")
	t.Setenv("MEMORY_MANAGERS_JSON", managerJSON)
	t.Setenv("MEMORY_WRAPPING_KEY", strings.Repeat("ab", 32))
	t.Setenv("MEMORY_MULTIRAG_URL", "http://127.0.0.1:8123")
	t.Setenv("MEMORY_MULTIRAG_KEY", "PRIVATE_BACKEND_CREDENTIAL")
	cfg, err := LoadConfig()
	require.NoError(t, err)
	require.Equal(t, managerJSON, cfg.Memory.ManagersJSON)
	values, err := cfg.Memory.Managers()
	require.NoError(t, err)
	require.Len(t, values, 1)
	require.Equal(t, manager.TenantID, values[0].HubTenantID)
	for _, tc := range []struct{ name, value string }{
		{"MEMORY_MANAGERS_JSON", `[{"token":"PRIVATE_INVALID_SECRET"}]`},
		{"MEMORY_CLIENTS_JSON", `[{"PRIVATE_UNKNOWN_PROPERTY":true}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.name, tc.value)
			_, err := LoadConfig()
			require.Error(t, err)
			require.NotContains(t, err.Error(), "PRIVATE_INVALID_SECRET")
			require.NotContains(t, err.Error(), "PRIVATE_UNKNOWN_PROPERTY")
		})
	}
}
