// Package config provides configuration loading for the MCP server.
package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the main configuration structure.
type Config struct {
	Server         ServerConfig         `yaml:"server"`
	Plugins        map[string]yaml.Node `yaml:"plugins"`
	Auth           AuthConfig           `yaml:"auth"`
	Sandbox        SandboxConfig        `yaml:"sandbox"`
	Proxy          ProxyConfig          `yaml:"proxy"`
	Storage        *StorageConfig       `yaml:"storage,omitempty"`
	Observability  ObservabilityConfig  `yaml:"observability"`
	SemanticSearch SemanticSearchConfig `yaml:"semantic_search"`
	RateLimit      RateLimitConfig      `yaml:"rate_limit"`
}

// AuthConfig holds authentication configuration.
type AuthConfig struct {
	Enabled     bool          `yaml:"enabled"`
	GitHub      *GitHubConfig `yaml:"github,omitempty"`
	AllowedOrgs []string      `yaml:"allowed_orgs,omitempty"`
	Tokens      TokensConfig  `yaml:"tokens"`
}

// GitHubConfig holds GitHub OAuth configuration.
type GitHubConfig struct {
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
}

// TokensConfig holds JWT token configuration.
type TokensConfig struct {
	SecretKey string `yaml:"secret_key"`
}

// ServerConfig holds server-specific configuration.
type ServerConfig struct {
	Host      string `yaml:"host"`
	Port      int    `yaml:"port"`
	BaseURL   string `yaml:"base_url"`
	Transport string `yaml:"transport"`
}

// SemanticSearchConfig holds configuration for semantic example search.
type SemanticSearchConfig struct {
	// ModelPath is the path to the GGUF embedding model file (required).
	ModelPath string `yaml:"model_path,omitempty"`

	// GPULayers is the number of layers to offload to GPU (0 = CPU only).
	GPULayers int `yaml:"gpu_layers,omitempty"`
}

// SandboxConfig holds sandbox execution configuration.
type SandboxConfig struct {
	Backend        string  `yaml:"backend"`
	Image          string  `yaml:"image"`
	Timeout        int     `yaml:"timeout"`
	MemoryLimit    string  `yaml:"memory_limit"`
	CPULimit       float64 `yaml:"cpu_limit"`
	Network        string  `yaml:"network"`
	HostSharedPath string  `yaml:"host_shared_path,omitempty"`

	// Session configuration for persistent execution environments.
	Sessions SessionConfig `yaml:"sessions"`

	// Logging configuration for sandbox executions.
	Logging SandboxLoggingConfig `yaml:"logging"`
}

// SandboxLoggingConfig holds logging configuration for sandbox executions.
type SandboxLoggingConfig struct {
	// LogCode logs the full Python code submitted to execute_python.
	// Disabled by default as code may contain sensitive data.
	LogCode bool `yaml:"log_code"`

	// LogOutput logs stdout and stderr from execution.
	// Disabled by default as output may be large or contain sensitive data.
	LogOutput bool `yaml:"log_output"`
}

// SessionConfig holds configuration for persistent sandbox sessions.
type SessionConfig struct {
	// Enabled controls whether session support is available. Defaults to true.
	Enabled *bool `yaml:"enabled,omitempty"`
	// TTL is the duration after which an idle session is destroyed (since last use).
	TTL time.Duration `yaml:"ttl"`
	// MaxDuration is the maximum lifetime of a session regardless of activity.
	MaxDuration time.Duration `yaml:"max_duration"`
	// MaxSessions is the maximum number of concurrent sessions allowed.
	MaxSessions int `yaml:"max_sessions"`
}

// IsEnabled returns whether sessions are enabled (defaults to true).
func (c *SessionConfig) IsEnabled() bool {
	if c.Enabled == nil {
		return true // Default to enabled
	}

	return *c.Enabled
}

// StorageConfig holds S3 storage configuration.
type StorageConfig struct {
	Endpoint        string `yaml:"endpoint"`
	AccessKey       string `yaml:"access_key"`
	SecretKey       string `yaml:"secret_key"`
	Bucket          string `yaml:"bucket"`
	Region          string `yaml:"region"`
	PublicURLPrefix string `yaml:"public_url_prefix,omitempty"`
}

// ObservabilityConfig holds observability configuration.
type ObservabilityConfig struct {
	MetricsEnabled bool `yaml:"metrics_enabled"`
	MetricsPort    int  `yaml:"metrics_port"`
}

// RateLimitConfig holds rate limiting configuration.
type RateLimitConfig struct {
	// Enabled controls whether rate limiting is active.
	Enabled bool `yaml:"enabled"`
	// Backend is the rate limiting backend: "memory" (default) or "redis".
	Backend string `yaml:"backend"`
	// Redis configuration (only used when backend is "redis").
	Redis *RateLimitRedisConfig `yaml:"redis,omitempty"`
	// Default limits applied to all tools/endpoints.
	Default RateLimitRule `yaml:"default"`
	// PerTool allows configuring different limits for specific tools.
	// Keys are tool names (e.g., "execute_python", "search_examples").
	PerTool map[string]RateLimitRule `yaml:"per_tool,omitempty"`
	// TrustedProxies is a list of IP addresses or CIDR ranges of trusted reverse proxies.
	// Used to determine the client IP from X-Forwarded-For headers.
	TrustedProxies []string `yaml:"trusted_proxies,omitempty"`
}

// RateLimitRule defines rate limit parameters for a tool or default.
type RateLimitRule struct {
	// RequestsPerSecond is the sustained rate limit (token bucket fill rate).
	RequestsPerSecond float64 `yaml:"requests_per_second"`
	// RequestsPerMinute is an alternative to RequestsPerSecond (will be converted).
	// If both are set, RequestsPerSecond takes precedence.
	RequestsPerMinute int `yaml:"requests_per_minute"`
	// BurstSize is the maximum burst size (bucket capacity).
	// Defaults to RequestsPerSecond if not set.
	BurstSize int `yaml:"burst_size"`
	// BlockDuration is how long to block requests after rate limit is exceeded.
	// Defaults to 60 seconds.
	BlockDuration time.Duration `yaml:"block_duration"`
}

// RateLimitRedisConfig holds Redis backend configuration for distributed rate limiting.
type RateLimitRedisConfig struct {
	// Address is the Redis server address (host:port).
	Address string `yaml:"address"`
	// Password for Redis authentication.
	Password string `yaml:"password"`
	// Database is the Redis database number.
	Database int `yaml:"database"`
	// KeyPrefix is the prefix for all rate limit keys in Redis.
	KeyPrefix string `yaml:"key_prefix"`
	// TLS enables TLS connection to Redis.
	TLS bool `yaml:"tls"`
}

// GetRequestsPerSecond returns the effective requests per second rate.
func (r RateLimitRule) GetRequestsPerSecond() float64 {
	if r.RequestsPerSecond > 0 {
		return r.RequestsPerSecond
	}
	if r.RequestsPerMinute > 0 {
		return float64(r.RequestsPerMinute) / 60.0
	}
	return 1.0 // Default: 1 request per second
}

// GetBurstSize returns the effective burst size.
func (r RateLimitRule) GetBurstSize() int {
	if r.BurstSize > 0 {
		return r.BurstSize
	}
	// Default burst is the ceiling of requests per second
	burst := int(r.GetRequestsPerSecond())
	if burst < 1 {
		return 1
	}
	return burst
}

// GetBlockDuration returns the effective block duration.
func (r RateLimitRule) GetBlockDuration() time.Duration {
	if r.BlockDuration > 0 {
		return r.BlockDuration
	}
	return 60 * time.Second
}

// ProxyConfig holds proxy connection configuration.
// The MCP server always connects to a proxy server (local or remote) via this config.
// Sandbox containers never receive credentials directly.
type ProxyConfig struct {
	// URL is the base URL of the proxy server (e.g., http://localhost:18081).
	// Required - the proxy must be running separately.
	URL string `yaml:"url"`

	// Auth configures authentication for the proxy.
	// Optional - if not set, the proxy must allow unauthenticated access.
	Auth *ProxyAuthConfig `yaml:"auth,omitempty"`
}

// ProxyAuthConfig configures authentication for the proxy.
type ProxyAuthConfig struct {
	// IssuerURL is the OIDC issuer URL for JWT authentication.
	IssuerURL string `yaml:"issuer_url"`

	// ClientID is the OAuth client ID for authentication.
	ClientID string `yaml:"client_id"`
}

// Load loads configuration from a YAML file with environment variable substitution.
func Load(path string) (*Config, error) {
	if path == "" {
		path = os.Getenv("CONFIG_PATH")
		if path == "" {
			path = "config.yaml"
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	// Substitute environment variables
	substituted, err := substituteEnvVars(string(data))
	if err != nil {
		return nil, fmt.Errorf("substituting env vars: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal([]byte(substituted), &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Apply defaults
	applyDefaults(&cfg)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &cfg, nil
}

// PluginConfigYAML returns the raw YAML bytes for a given plugin name.
// Returns nil if the plugin is not configured.
func (c *Config) PluginConfigYAML(name string) ([]byte, error) {
	node, ok := c.Plugins[name]
	if !ok {
		return nil, nil
	}

	data, err := yaml.Marshal(&node)
	if err != nil {
		return nil, fmt.Errorf("marshaling plugin %q config: %w", name, err)
	}

	return data, nil
}

// envVarWithDefaultPattern matches ${VAR_NAME:-default} patterns.
var envVarWithDefaultPattern = regexp.MustCompile(`\$\{([^}:]+)(?::-([^}]*))?\}`)

// substituteEnvVars replaces ${VAR_NAME} and ${VAR_NAME:-default} patterns with environment variable values.
// Lines that are comments (starting with #) are skipped.
// Missing environment variables without defaults are replaced with empty strings (lenient mode).
func substituteEnvVars(content string) (string, error) {
	lines := strings.Split(content, "\n")

	for i, line := range lines {
		// Skip lines that are YAML comments.
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		lines[i] = envVarWithDefaultPattern.ReplaceAllStringFunc(line, func(match string) string {
			parts := envVarWithDefaultPattern.FindStringSubmatch(match)
			varName := parts[1]
			defaultVal := ""
			if len(parts) > 2 {
				defaultVal = parts[2]
			}

			value := os.Getenv(varName)
			if value == "" {
				return defaultVal // Use default or empty string
			}

			return value
		})
	}

	return strings.Join(lines, "\n"), nil
}

// applyDefaults sets default values for configuration fields.
func applyDefaults(cfg *Config) {
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}

	if cfg.Server.Port == 0 {
		cfg.Server.Port = 2480
	}

	if cfg.Server.Transport == "" {
		cfg.Server.Transport = "stdio"
	}

	if cfg.Sandbox.Backend == "" {
		cfg.Sandbox.Backend = "docker"
	}

	if cfg.Sandbox.Timeout == 0 {
		cfg.Sandbox.Timeout = 60
	}

	if cfg.Sandbox.MemoryLimit == "" {
		cfg.Sandbox.MemoryLimit = "2g"
	}

	if cfg.Sandbox.CPULimit == 0 {
		cfg.Sandbox.CPULimit = 1.0
	}

	// Session defaults.
	if cfg.Sandbox.Sessions.TTL == 0 {
		cfg.Sandbox.Sessions.TTL = 30 * time.Minute
	}

	if cfg.Sandbox.Sessions.MaxDuration == 0 {
		cfg.Sandbox.Sessions.MaxDuration = 4 * time.Hour
	}

	if cfg.Sandbox.Sessions.MaxSessions == 0 {
		cfg.Sandbox.Sessions.MaxSessions = 10
	}

	if cfg.Observability.MetricsPort == 0 {
		cfg.Observability.MetricsPort = 2490
	}

	// Rate limit defaults.
	if cfg.RateLimit.Backend == "" {
		cfg.RateLimit.Backend = "memory"
	}
	if cfg.RateLimit.Default.RequestsPerSecond == 0 && cfg.RateLimit.Default.RequestsPerMinute == 0 {
		cfg.RateLimit.Default.RequestsPerSecond = 10 // Default: 10 req/s
	}
	if cfg.RateLimit.Default.BurstSize == 0 {
		cfg.RateLimit.Default.BurstSize = 20 // Default: burst of 20
	}

	// Proxy defaults.
	if cfg.Proxy.URL == "" {
		cfg.Proxy.URL = "http://localhost:18081"
	}

	// Semantic search defaults.
	if cfg.SemanticSearch.ModelPath == "" {
		// Prefer local dev path if present, otherwise fall back to container path.
		localPath := "models/MiniLM-L6-v2.Q8_0.gguf"
		containerPath := "/usr/share/mcp/MiniLM-L6-v2.Q8_0.gguf"

		switch {
		case fileExists(localPath):
			cfg.SemanticSearch.ModelPath = localPath
		case fileExists(containerPath):
			cfg.SemanticSearch.ModelPath = containerPath
		default:
			cfg.SemanticSearch.ModelPath = localPath
		}
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// MaxSandboxTimeout is the maximum allowed sandbox timeout in seconds.
const MaxSandboxTimeout = 600

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.Sandbox.Image == "" {
		return errors.New("sandbox.image is required")
	}

	// Validate sandbox timeout is within bounds.
	if c.Sandbox.Timeout > MaxSandboxTimeout {
		return fmt.Errorf("sandbox.timeout cannot exceed %d seconds", MaxSandboxTimeout)
	}

	return nil
}
