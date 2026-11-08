package config

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemoryConfigurationIsOptInAndFailClosed(t *testing.T) {
	clients, err := (MemoryConfig{}).Clients()
	require.NoError(t, err)
	require.Empty(t, clients)
	base := MemoryConfig{Enabled: true, WrappingKey: strings.Repeat("ab", 32), MultiragURL: "http://127.0.0.1:9380", MultiragKey: "private-service-key"}
	c := MemoryClient{Token: strings.Repeat("t", 32), TenantID: "example-tenant", ApplicationID: "memory-app", DatasetID: "private-dataset", PolicyVersion: "synthetic-v1"}
	b, _ := json.Marshal([]MemoryClient{c})
	base.ClientsJSON = string(b)
	clients, err = base.Clients()
	require.NoError(t, err)
	require.Len(t, clients, 1)
	delegated := c
	delegated.HubTenantID = "hub-default"
	b, _ = json.Marshal([]MemoryClient{delegated})
	base.ClientsJSON = string(b)
	clients, err = base.Clients()
	require.NoError(t, err)
	require.Equal(t, "hub-default", clients[0].HubTenantID)
	for name, mutate := range map[string]func(*MemoryConfig){
		"tenant audit width": func(m *MemoryConfig) {
			bad := c
			bad.TenantID = strings.Repeat("a", 65)
			b, _ := json.Marshal([]MemoryClient{bad})
			m.ClientsJSON = string(b)
		},
		"token whitespace": func(m *MemoryConfig) {
			bad := c
			bad.Token = strings.Repeat(" ", 32)
			b, _ := json.Marshal([]MemoryClient{bad})
			m.ClientsJSON = string(b)
		},
		"invalid delegated tenant": func(m *MemoryConfig) {
			bad := c
			bad.HubTenantID = "wrong/tenant"
			b, _ := json.Marshal([]MemoryClient{bad})
			m.ClientsJSON = string(b)
		},
		"delegated tenant width": func(m *MemoryConfig) {
			bad := c
			bad.HubTenantID = strings.Repeat("a", 65)
			b, _ := json.Marshal([]MemoryClient{bad})
			m.ClientsJSON = string(b)
		},
		"automatic extraction on knowledge": func(m *MemoryConfig) {
			bad := c
			bad.AutomaticExtraction = true
			b, _ := json.Marshal([]MemoryClient{bad})
			m.ClientsJSON = string(b)
		},
		"missing key":                func(m *MemoryConfig) { m.WrappingKey = "" },
		"missing backend credential": func(m *MemoryConfig) { m.MultiragKey = "" },
		"credential in url":          func(m *MemoryConfig) { m.MultiragURL = "https://secret@example.com" },
		"unknown client property":    func(m *MemoryConfig) { m.ClientsJSON = `[{"realData":true}]` },
		"duplicate app":              func(m *MemoryConfig) { b, _ := json.Marshal([]MemoryClient{c, c}); m.ClientsJSON = string(b) },
		"trailing JSON":              func(m *MemoryConfig) { m.ClientsJSON += `{}` },
	} {
		t.Run(name, func(t *testing.T) { bad := base; mutate(&bad); _, err := bad.Clients(); require.Error(t, err) })
	}
}

func TestMemoryNativeConfigurationUsesExplicitMemoryBank(t *testing.T) {
	base := MemoryConfig{Enabled: true, WrappingKey: strings.Repeat("ab", 32), MultiragURL: "http://127.0.0.1:8123", MultiragKey: "private-service-key"}
	c := MemoryClient{Token: strings.Repeat("t", 32), TenantID: "example-tenant", ApplicationID: "memory-app", Backend: "multirag-memory", MemoryID: "native-memory-bank", PolicyVersion: "synthetic-v1"}
	b, err := json.Marshal([]MemoryClient{c})
	require.NoError(t, err)
	base.ClientsJSON = string(b)
	clients, err := base.Clients()
	require.NoError(t, err)
	require.Equal(t, c.MemoryID, clients[0].MemoryID)
	c.AutomaticExtraction = true
	encoded, _ := json.Marshal([]MemoryClient{c})
	base.ClientsJSON = string(encoded)
	clients, err = base.Clients()
	require.NoError(t, err)
	require.True(t, clients[0].AutomaticExtraction)
	for name, mutate := range map[string]func(*MemoryClient){
		"missing bank":              func(client *MemoryClient) { client.MemoryID = "" },
		"both containers":           func(client *MemoryClient) { client.DatasetID = "dataset" },
		"unknown backend":           func(client *MemoryClient) { client.Backend = "native-auto-fallback" },
		"bank on knowledge backend": func(client *MemoryClient) { client.Backend = "multirag-knowledge"; client.DatasetID = "dataset" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := c
			mutate(&bad)
			encoded, marshalErr := json.Marshal([]MemoryClient{bad})
			require.NoError(t, marshalErr)
			cfg := base
			cfg.ClientsJSON = string(encoded)
			_, err := cfg.Clients()
			require.Error(t, err)
		})
	}
}
