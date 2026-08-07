package config

import (
	"os"
	"testing"
)

func loadConfigWithEnv(env map[string]string) (*Config, error) {
	for key, value := range env {
		os.Setenv(key, value)
	}
	defer func() {
		for key := range env {
			os.Unsetenv(key)
		}
	}()
	return LoadConfig()
}

func TestDeployerConfig(t *testing.T) {
	cfg, err := loadConfigWithEnv(map[string]string{
		"AGENT_DEPLOYER_URL":         "http://deployer:8080/api/v1",
		"AGENT_DEPLOYER_API_KEY":     "secret",
		"AGENT_DEPLOYER_PUBLIC_HOST": "deploy.example.com",
	})
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	if cfg.Deployer.URL != "http://deployer:8080/api/v1" {
		t.Errorf("Deployer.URL = %q, want %q", cfg.Deployer.URL, "http://deployer:8080/api/v1")
	}
	if cfg.Deployer.APIKey != "secret" {
		t.Errorf("Deployer.APIKey = %q, want %q", cfg.Deployer.APIKey, "secret")
	}
	if cfg.Deployer.PublicHost != "deploy.example.com" {
		t.Errorf("Deployer.PublicHost = %q, want %q", cfg.Deployer.PublicHost, "deploy.example.com")
	}
}

func TestLoadConfig_RuntimeAPIKey(t *testing.T) {
	cfg, err := loadConfigWithEnv(map[string]string{
		"AGENT_DEPLOYER_URL":    "http://deployer:8080/api/v1",
		"AGENT_RUNTIME_API_KEY": "rt-secret-123",
	})
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	if cfg.Deployer.RuntimeAPIKey != "rt-secret-123" {
		t.Errorf("RuntimeAPIKey = %q, want %q", cfg.Deployer.RuntimeAPIKey, "rt-secret-123")
	}
}

func TestKnowledgeConfig(t *testing.T) {
	cfg, err := loadConfigWithEnv(map[string]string{
		"MULTIRAG_BASE_URL":               "http://multirag:8000",
		"MULTIRAG_API_KEY":                "service-token",
		"MULTIRAG_TIMEOUT_SECONDS":        "45",
		"MULTIRAG_UPLOAD_TIMEOUT_SECONDS": "1800",
	})
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	if cfg.Knowledge.MultiragBaseURL != "http://multirag:8000" {
		t.Errorf("MultiragBaseURL = %q, want %q", cfg.Knowledge.MultiragBaseURL, "http://multirag:8000")
	}
	if cfg.Knowledge.MultiragAPIKey != "service-token" {
		t.Errorf("MultiragAPIKey = %q, want %q", cfg.Knowledge.MultiragAPIKey, "service-token")
	}
	if cfg.Knowledge.TimeoutSeconds != 45 {
		t.Errorf("TimeoutSeconds = %d, want 45", cfg.Knowledge.TimeoutSeconds)
	}
	if cfg.Knowledge.UploadTimeoutSeconds != 1800 {
		t.Errorf("UploadTimeoutSeconds = %d, want 1800", cfg.Knowledge.UploadTimeoutSeconds)
	}
}

func TestKnowledgeConfig_DefaultsAndMissingAllowed(t *testing.T) {
	os.Unsetenv("MULTIRAG_BASE_URL")
	os.Unsetenv("MULTIRAG_API_KEY")
	os.Unsetenv("MULTIRAG_TIMEOUT_SECONDS")
	os.Unsetenv("MULTIRAG_UPLOAD_TIMEOUT_SECONDS")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	if cfg.Knowledge.MultiragBaseURL != "" {
		t.Errorf("MultiragBaseURL = %q, want empty", cfg.Knowledge.MultiragBaseURL)
	}
	if cfg.Knowledge.MultiragAPIKey != "" {
		t.Errorf("MultiragAPIKey = %q, want empty", cfg.Knowledge.MultiragAPIKey)
	}
	if cfg.Knowledge.TimeoutSeconds != 30 {
		t.Errorf("TimeoutSeconds = %d, want 30", cfg.Knowledge.TimeoutSeconds)
	}
	if cfg.Knowledge.UploadTimeoutSeconds != 3600 {
		t.Errorf("UploadTimeoutSeconds = %d, want 3600", cfg.Knowledge.UploadTimeoutSeconds)
	}
}
