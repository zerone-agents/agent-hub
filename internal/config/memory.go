package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// Memory is an opt-in, synthetic-data integration service. Production admission
// (including KMS and protection of the retrieval replica) is deliberately not a
// configurable bypass in this release.
type MemoryConfig struct {
	Enabled     bool   `mapstructure:"enabled"`
	ClientsJSON string `mapstructure:"clients_json"`
	WrappingKey string `mapstructure:"wrapping_key"`
	MultiragURL string `mapstructure:"multirag_url"`
	MultiragKey string `mapstructure:"multirag_key"`
}

type MemoryClient struct {
	Token               string `json:"token"`
	TenantID            string `json:"tenantId"`
	ApplicationID       string `json:"applicationId"`
	DatasetID           string `json:"datasetId,omitempty"`
	MemoryID            string `json:"memoryId,omitempty"`
	Backend             string `json:"backend,omitempty"`
	AutomaticExtraction bool   `json:"automaticExtraction,omitempty"`
	PolicyVersion       string `json:"policyVersion"`
	// HubTenantID explicitly delegates administration to a Hub tenant. Empty
	// values retain the memory tenant as the administration tenant.
	HubTenantID string `json:"hubTenantId,omitempty"`
}

func (m MemoryConfig) Clients() ([]MemoryClient, error) {
	if !m.Enabled {
		return nil, nil
	}
	key, err := hex.DecodeString(m.WrappingKey)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("MEMORY_WRAPPING_KEY must be 64 hexadecimal characters")
	}
	u, err := url.Parse(m.MultiragURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || m.MultiragKey == "" {
		return nil, fmt.Errorf("memory requires a valid MULTIRAG URL and service credential")
	}
	var clients []MemoryClient
	dec := json.NewDecoder(strings.NewReader(m.ClientsJSON))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&clients); err != nil || len(clients) == 0 || len(clients) > 32 {
		return nil, fmt.Errorf("MEMORY_CLIENTS_JSON must contain 1 to 32 service clients")
	}
	var trailing any
	if dec.Decode(&trailing) != io.EOF {
		return nil, fmt.Errorf("MEMORY_CLIENTS_JSON must contain one JSON value")
	}
	apps, datasets, tokens := map[string]bool{}, map[string]bool{}, map[[32]byte]bool{}
	for _, c := range clients {
		app := c.TenantID + "\x00" + c.ApplicationID
		token := sha256.Sum256([]byte(c.Token))
		backend, container := c.Backend, c.DatasetID
		validContainer := false
		switch backend {
		case "", "multirag-knowledge":
			backend = "multirag-knowledge"
			validContainer = memoryDatasetID(c.DatasetID) && c.MemoryID == "" && !c.AutomaticExtraction
		case "multirag-memory":
			container = c.MemoryID
			validContainer = memoryDatasetID(c.MemoryID) && c.DatasetID == ""
		}
		containerKey := backend + "\x00" + strings.ToLower(container)
		if !memoryToken(c.Token) || len(c.TenantID) > 64 || !memoryIdentifier(c.TenantID) || (c.HubTenantID != "" && (len(c.HubTenantID) > 64 || !memoryIdentifier(c.HubTenantID))) || !memoryIdentifier(c.ApplicationID) || !validContainer || !memoryIdentifier(c.PolicyVersion) || apps[app] || datasets[containerKey] || tokens[token] {
			return nil, fmt.Errorf("memory clients require unique application, backend container and token, valid identifiers and tokens of at least 32 bytes")
		}
		apps[app], datasets[containerKey], tokens[token] = true, true, true
	}
	return clients, nil
}

func memoryToken(s string) bool {
	if len(s) < 32 || len(s) > 512 {
		return false
	}
	for _, r := range s {
		if r < 0x21 || r > 0x7e {
			return false
		}
	}
	return true
}

func memoryDatasetID(s string) bool {
	if len(s) == 0 || len(s) > 32 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func memoryIdentifier(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}
