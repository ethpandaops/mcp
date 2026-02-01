package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		envVars     map[string]string
		expectError bool
	}{
		{
			name: "valid minimal config",
			content: `
server:
  host: 0.0.0.0
  port: 2480
sandbox:
  image: test-image:latest
`,
			expectError: false,
		},
		{
			name: "config with env substitution",
			content: `
server:
  host: 0.0.0.0
  port: ${PORT:-2480}
sandbox:
  image: ${SANDBOX_IMAGE:-default:latest}
`,
			expectError: false,
		},
		{
			name: "config with missing sandbox image",
			content: `
server:
  host: 0.0.0.0
  port: 2480
`,
			expectError: true,
		},
		{
			name: "config with timeout too high",
			content: `
server:
  host: 0.0.0.0
  port: 2480
sandbox:
  image: test-image:latest
  timeout: 601
`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")
			err := os.WriteFile(configPath, []byte(tt.content), 0644)
			require.NoError(t, err)

			// Clear env vars that might interfere
			os.Unsetenv("PORT")
			os.Unsetenv("SANDBOX_IMAGE")

			cfg, err := Load(configPath)
			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, cfg)
		})
	}
}

func TestLoadWithEnvVars(t *testing.T) {
	content := `
server:
  host: 0.0.0.0
  port: ${TEST_PORT:-3000}
sandbox:
  image: ${TEST_IMAGE:-fallback:latest}
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte(content), 0644)
	require.NoError(t, err)

	// Test with env var set
	t.Setenv("TEST_PORT", "9999")
	t.Setenv("TEST_IMAGE", "custom:latest")

	cfg, err := Load(configPath)
	require.NoError(t, err)
	assert.Equal(t, 9999, cfg.Server.Port)
	assert.Equal(t, "custom:latest", cfg.Sandbox.Image)
}

func TestApplyDefaults(t *testing.T) {
	cfg := &Config{
		Sandbox: SandboxConfig{
			Image: "test:latest",
		},
	}

	applyDefaults(cfg)

	assert.Equal(t, "0.0.0.0", cfg.Server.Host)
	assert.Equal(t, 2480, cfg.Server.Port)
	assert.Equal(t, "stdio", cfg.Server.Transport)
	assert.Equal(t, "docker", cfg.Sandbox.Backend)
	assert.Equal(t, 60, cfg.Sandbox.Timeout)
	assert.Equal(t, "2g", cfg.Sandbox.MemoryLimit)
	assert.Equal(t, 1.0, cfg.Sandbox.CPULimit)
	assert.Equal(t, 30*time.Minute, cfg.Sandbox.Sessions.TTL)
	assert.Equal(t, 4*time.Hour, cfg.Sandbox.Sessions.MaxDuration)
	assert.Equal(t, 10, cfg.Sandbox.Sessions.MaxSessions)
	assert.Equal(t, 2490, cfg.Observability.MetricsPort)
	assert.Equal(t, "http://localhost:18081", cfg.Proxy.URL)
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name        string
		cfg         Config
		expectError bool
	}{
		{
			name: "valid config",
			cfg: Config{
				Sandbox: SandboxConfig{
					Image:   "test:latest",
					Timeout: 60,
				},
			},
			expectError: false,
		},
		{
			name: "missing image",
			cfg: Config{
				Sandbox: SandboxConfig{
					Timeout: 60,
				},
			},
			expectError: true,
		},
		{
			name: "timeout exceeds max",
			cfg: Config{
				Sandbox: SandboxConfig{
					Image:   "test:latest",
					Timeout: 601,
				},
			},
			expectError: true,
		},
		{
			name: "timeout at max boundary",
			cfg: Config{
				Sandbox: SandboxConfig{
					Image:   "test:latest",
					Timeout: 600,
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSessionConfigIsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		enabled  *bool
		expected bool
	}{
		{
			name:     "nil defaults to true",
			enabled:  nil,
			expected: true,
		},
		{
			name:     "explicitly true",
			enabled:  boolPtr(true),
			expected: true,
		},
		{
			name:     "explicitly false",
			enabled:  boolPtr(false),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := SessionConfig{Enabled: tt.enabled}
			assert.Equal(t, tt.expected, cfg.IsEnabled())
		})
	}
}

func TestPluginConfigYAML(t *testing.T) {
	// Create a config file and load it to properly initialize the Plugins map
	content := `
server:
  host: 0.0.0.0
  port: 2480
sandbox:
  image: test-image:latest
plugins:
  clickhouse:
    clusters:
      - name: test
        host: localhost
        database: testdb
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := Load(configPath)
	require.NoError(t, err)

	// Test getting existing plugin config
	data, err := cfg.PluginConfigYAML("clickhouse")
	assert.NoError(t, err)
	assert.NotNil(t, data)
	assert.Contains(t, string(data), "test")

	// Test getting non-existent plugin config
	data, err = cfg.PluginConfigYAML("nonexistent")
	assert.NoError(t, err)
	assert.Nil(t, data)
}

func TestSubstituteEnvVars(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		envVars  map[string]string
		expected string
	}{
		{
			name:     "no substitution needed",
			content:  "key: value",
			expected: "key: value",
		},
		{
			name:     "simple substitution",
			content:  "key: ${TEST_VAR}",
			envVars:  map[string]string{"TEST_VAR": "replaced"},
			expected: "key: replaced",
		},
		{
			name:     "substitution with default",
			content:  "key: ${MISSING_VAR:-default_value}",
			expected: "key: default_value",
		},
		{
			name:     "comment lines skipped",
			content:  "# ${IGNORED}\nkey: value",
			expected: "# ${IGNORED}\nkey: value",
		},
		{
			name:     "multiple substitutions",
			content:  "a: ${VAR1}\nb: ${VAR2:-default}",
			envVars:  map[string]string{"VAR1": "one"},
			expected: "a: one\nb: default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set env vars
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}
			// Clear vars not in test
			if _, exists := tt.envVars["TEST_VAR"]; !exists {
				os.Unsetenv("TEST_VAR")
			}
			if _, exists := tt.envVars["VAR1"]; !exists {
				os.Unsetenv("VAR1")
			}

			result, err := substituteEnvVars(tt.content)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFileExists(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(tmpDir, "exists.txt")
	err := os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	assert.True(t, fileExists(testFile))
	assert.False(t, fileExists(filepath.Join(tmpDir, "nonexistent.txt")))
}

// Helper functions
func boolPtr(b bool) *bool {
	return &b
}
