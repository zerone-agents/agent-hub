package config

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/viper"
)

var globalConfig *Config

// SetGlobalConfig stores the loaded configuration for global access.
func SetGlobalConfig(cfg *Config) {
	globalConfig = cfg
}

// Get returns the global configuration. It returns nil if the configuration has
// not been loaded.
func Get() *Config {
	return globalConfig
}

type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Auth      AuthConfig      `mapstructure:"auth"`
	Casdoor   CasdoorConfig   `mapstructure:"casdoor"`
	OSS       OSSConfig       `mapstructure:"oss"`
	Provider  ProviderConfig  `mapstructure:"provider"`
	Deployer  DeployerConfig  `mapstructure:"deployer"`
	Knowledge KnowledgeConfig `mapstructure:"knowledge"`
	Kong      KongConfig      `mapstructure:"kong"`
}

// AuthConfig selects the authentication backend. Mode "builtin" (default) uses
// the built-in username/password user system; "casdoor" delegates to the
// existing Casdoor OAuth integration. JWTSecret is required for builtin mode
// and must be at least 32 bytes.
type AuthConfig struct {
	Mode      string `mapstructure:"mode"`
	JWTSecret string `mapstructure:"jwt_secret"`
}

// IsBuiltin reports whether the builtin auth backend is active.
func (a *AuthConfig) IsBuiltin() bool { return a.Mode == "builtin" }

// IsCasdoor reports whether the Casdoor auth backend is active.
func (a *AuthConfig) IsCasdoor() bool { return a.Mode == "casdoor" }

// ValidateAuth enforces auth config invariants: Mode must be one of the
// supported values, and builtin mode requires a JWTSecret of at least 32 bytes.
func (c *Config) ValidateAuth() error {
	switch c.Auth.Mode {
	case "builtin":
		if len(c.Auth.JWTSecret) < 32 {
			return fmt.Errorf("auth.jwt_secret 必填且至少 32 字节（builtin 模式）")
		}
	case "casdoor":
		// Casdoor 使用自身签名的 token 校验，无需本地 JWT secret；
		// 角色已改为 agent-hub 本地管理，不再有 role mapping / default role 配置。
		// Organization 可选：仅作为存量数据回填目标的显式覆盖（一次性升级
		// 逃生舱）。未配置时 AutoMigrate 从 user_identities 自动推断；仅在
		// 存量数据无法归属（0 或多个租户）时才需要临时配置。
	default:
		return fmt.Errorf("auth.mode 必须是 builtin 或 casdoor，当前: %q", c.Auth.Mode)
	}
	return nil
}

type DeployerConfig struct {
	URL           string `mapstructure:"url"`
	APIKey        string `mapstructure:"api_key"`
	PublicHost    string `mapstructure:"public_host"`
	UpstreamHost  string `mapstructure:"upstream_host"`
	RuntimeAPIKey string `mapstructure:"runtime_api_key"`
}

type KongConfig struct {
	AdminURL     string `mapstructure:"admin_url"`     // Kong Admin API；空 = 禁用 Kong 集成（no-op）
	ReconcileSec int    `mapstructure:"reconcile_sec"` // 对账间隔秒，默认 300
}

type KnowledgeConfig struct {
	MultiragBaseURL      string `mapstructure:"multirag_base_url"`
	MultiragAPIKey       string `mapstructure:"multirag_api_key"`
	TimeoutSeconds       int    `mapstructure:"timeout_seconds"`
	UploadTimeoutSeconds int    `mapstructure:"upload_timeout_seconds"`
}

type ProviderConfig struct {
	EncryptionKey string `mapstructure:"encryption_key"`
}

type OSSConfig struct {
	Endpoint       string `mapstructure:"endpoint"`
	Region         string `mapstructure:"region"`
	Bucket         string `mapstructure:"bucket"`
	AccessKey      string `mapstructure:"access_key"`
	SecretKey      string `mapstructure:"secret_key"`
	ForcePathStyle bool   `mapstructure:"force_path_style"`
	CDNHost        string `mapstructure:"cdn_host"`
}

type ServerConfig struct {
	Host        string   `mapstructure:"host"`
	Port        int      `mapstructure:"port"`
	CorsOrigins []string `mapstructure:"cors_origins"`
}

type DatabaseConfig struct {
	URL         string `mapstructure:"url"`
	MaxIdle     int    `mapstructure:"max_idle"`
	MaxOpen     int    `mapstructure:"max_open"`
	MaxLifetime int    `mapstructure:"max_lifetime"`
}

type CasdoorConfig struct {
	Endpoint     string `mapstructure:"endpoint"`
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	Certificate  string `mapstructure:"certificate"`
	Organization string `mapstructure:"organization"`
	CallbackURL  string `mapstructure:"callback_url"`
}

func LoadConfig() (*Config, error) {
	viper.AutomaticEnv()
	viper.SetEnvPrefix("")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	bindEnvVars := []string{
		"server.host", "server.port", "server.cors_origins",
		"database.url", "database.max_idle", "database.max_open", "database.max_lifetime",
		"casdoor.endpoint", "casdoor.client_id", "casdoor.client_secret", "casdoor.certificate",
		"casdoor.organization", "casdoor.callback_url",
		"auth.mode", "auth.jwt_secret",
		"oss.endpoint", "oss.region", "oss.bucket", "oss.access_key", "oss.secret_key", "oss.force_path_style", "oss.cdn_host",
		"provider.encryption_key",
	}
	for _, key := range bindEnvVars {
		viper.BindEnv(key)
	}
	viper.BindEnv("deployer.url", "AGENT_DEPLOYER_URL")
	viper.BindEnv("deployer.api_key", "AGENT_DEPLOYER_API_KEY")
	viper.BindEnv("deployer.public_host", "AGENT_DEPLOYER_PUBLIC_HOST")
	viper.BindEnv("deployer.runtime_api_key", "AGENT_RUNTIME_API_KEY")
	viper.BindEnv("knowledge.multirag_base_url", "MULTIRAG_BASE_URL")
	viper.BindEnv("knowledge.multirag_api_key", "MULTIRAG_API_KEY")
	viper.BindEnv("knowledge.timeout_seconds", "MULTIRAG_TIMEOUT_SECONDS")
	viper.BindEnv("knowledge.upload_timeout_seconds", "MULTIRAG_UPLOAD_TIMEOUT_SECONDS")
	viper.BindEnv("kong.admin_url", "KONG_ADMIN_URL")
	viper.BindEnv("kong.reconcile_sec", "KONG_RECONCILE_SEC")

	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 8081)
	viper.SetDefault("auth.mode", "builtin")
	viper.SetDefault("database.max_idle", 10)
	viper.SetDefault("database.max_open", 100)
	viper.SetDefault("database.max_lifetime", 3600)
	viper.SetDefault("knowledge.timeout_seconds", 30)
	viper.SetDefault("knowledge.upload_timeout_seconds", 3600)
	viper.SetDefault("kong.reconcile_sec", 300)

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// 角色已改为 agent-hub 本地管理，这两个环境变量不再生效；
	// 检测到仅打 warning 不 fail，给线上留清理窗口。
	for _, name := range []string{"CASDOOR_ROLE_MAPPING", "CASDOOR_DEFAULT_ROLE"} {
		if os.Getenv(name) != "" {
			log.Printf("[config] WARNING: %s 已废弃，角色现由 agent-hub 本地管理，请从环境变量中移除", name)
		}
	}

	// If no public host is configured for runtime URLs, fall back to the
	// hostname parsed from the agent-deployer URL.
	if cfg.Deployer.PublicHost == "" && cfg.Deployer.URL != "" {
		if u, err := url.Parse(cfg.Deployer.URL); err == nil && u.Hostname() != "" {
			cfg.Deployer.PublicHost = u.Hostname()
		}
	}

	// Kong service upstream host defaults to the deployer URL hostname (internal
	// address Kong uses to reach the runtime containers). If explicitly configured,
	// PublicHost can still be used as a fallback for backwards compatibility.
	if cfg.Deployer.UpstreamHost == "" && cfg.Deployer.URL != "" {
		if u, err := url.Parse(cfg.Deployer.URL); err == nil && u.Hostname() != "" {
			cfg.Deployer.UpstreamHost = u.Hostname()
		}
	}
	if cfg.Deployer.UpstreamHost == "" && cfg.Deployer.PublicHost != "" {
		cfg.Deployer.UpstreamHost = cfg.Deployer.PublicHost
	}

	SetGlobalConfig(&cfg)
	return &cfg, nil
}

func (c *Config) GetServerAddr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}
