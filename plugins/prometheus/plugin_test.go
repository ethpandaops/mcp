package prometheus

import (
	"testing"

	"github.com/ethpandaops/mcp/pkg/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	p := New()
	require.NotNil(t, p)
	assert.Equal(t, "prometheus", p.Name())
}

func TestPluginInit(t *testing.T) {
	tests := []struct {
		name        string
		config      string
		expectError bool
		expectSkip  bool
	}{
		{
			name: "valid config with instances",
			config: `
instances:
  - name: test-prom
    url: http://localhost:9090
`,
			expectError: false,
			expectSkip:  false,
		},
		{
			name: "config filters invalid instances keeps valid ones",
			config: `
instances:
  - name: valid
    url: http://localhost:9090
  - name: ""
    url: http://localhost:9091
  - name: missing-url
    url: ""
`,
			expectError: false, // valid instance is kept, invalid ones filtered
			expectSkip:  false,
		},
		{
			name:        "empty config returns ErrNoValidConfig",
			config:      "",
			expectError: true,
			expectSkip:  true,
		},
		{
			name: "config with only invalid instances returns ErrNoValidConfig",
			config: `
instances:
  - name: ""
    url: http://localhost:9090
  - name: test
    url: ""
`,
			expectError: true,
			expectSkip:  true,
		},
		{
			name:        "invalid yaml",
			config:      `instances: [invalid`,
			expectError: true,
			expectSkip:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New()
			err := p.Init([]byte(tt.config))
			if tt.expectError {
				assert.Error(t, err)
				if tt.expectSkip {
					assert.ErrorIs(t, err, plugin.ErrNoValidConfig)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPluginApplyDefaults(t *testing.T) {
	p := New()

	err := p.Init([]byte(`
instances:
  - name: test
    url: http://localhost:9090
`))
	require.NoError(t, err)

	p.ApplyDefaults()

	assert.Equal(t, 60, p.cfg.Instances[0].Timeout)
}

func TestPluginValidate(t *testing.T) {
	tests := []struct {
		name        string
		config      string
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid config",
			config: `
instances:
  - name: prom1
    url: http://localhost:9090
`,
			expectError: false,
		},
		{
			name: "multiple valid instances",
			config: `
instances:
  - name: prom1
    url: http://localhost:9090
  - name: prom2
    url: http://localhost:9091
`,
			expectError: false,
		},
		{
			name: "missing instance name",
			config: `
instances:
  - url: http://localhost:9090
`,
			expectError: true,
			errorMsg:    "instances[0].name is required",
		},
		{
			name: "duplicate instance names",
			config: `
instances:
  - name: same
    url: http://localhost:9090
  - name: same
    url: http://localhost:9091
`,
			expectError: true,
			errorMsg:    "duplicated",
		},
		{
			name: "missing instance URL",
			config: `
instances:
  - name: prom1
`,
			expectError: true,
			errorMsg:    "instances[0].url is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New()
			err := p.Init([]byte(tt.config))
			if err != nil {
				// If Init returns ErrNoValidConfig, skip validation test
				if err == plugin.ErrNoValidConfig {
					return
				}
				t.Fatalf("Init failed: %v", err)
			}

			err = p.Validate()
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPluginSandboxEnv(t *testing.T) {
	tests := []struct {
		name         string
		config       string
		expectedKeys int
	}{
		{
			name: "with instances",
			config: `
instances:
  - name: prom1
    url: http://localhost:9090
    description: Test Prometheus
`,
			expectedKeys: 1,
		},
		{
			name:         "no instances",
			config:       "",
			expectedKeys: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New()
			err := p.Init([]byte(tt.config))
			if err == plugin.ErrNoValidConfig {
				env, err := p.SandboxEnv()
				require.NoError(t, err)
				assert.Equal(t, 0, len(env))
				return
			}
			require.NoError(t, err)

			env, err := p.SandboxEnv()
			require.NoError(t, err)
			assert.Equal(t, tt.expectedKeys, len(env))

			if tt.expectedKeys > 0 {
				assert.Contains(t, env, "ETHPANDAOPS_PROMETHEUS_DATASOURCES")
			}
		})
	}
}

func TestPluginDatasourceInfo(t *testing.T) {
	p := New()
	err := p.Init([]byte(`
instances:
  - name: prom1
    url: http://localhost:9090
    description: First Prometheus
  - name: prom2
    url: http://localhost:9091
    description: Second Prometheus
`))
	require.NoError(t, err)

	infos := p.DatasourceInfo()
	assert.Equal(t, 2, len(infos))
	assert.Equal(t, "prometheus", infos[0].Type)
	assert.Equal(t, "prom1", infos[0].Name)
	assert.Equal(t, "http://localhost:9090", infos[0].Metadata["url"])
}

func TestPluginExamples(t *testing.T) {
	p := New()
	examples := p.Examples()
	assert.NotEmpty(t, examples)
}

func TestPluginPythonAPIDocs(t *testing.T) {
	p := New()
	docs := p.PythonAPIDocs()
	assert.Contains(t, docs, "prometheus")
	assert.NotEmpty(t, docs["prometheus"].Description)
	assert.NotEmpty(t, docs["prometheus"].Functions)

	// Check specific functions
	assert.Contains(t, docs["prometheus"].Functions, "query")
	assert.Contains(t, docs["prometheus"].Functions, "query_range")
	assert.Contains(t, docs["prometheus"].Functions, "get_labels")
	assert.Contains(t, docs["prometheus"].Functions, "get_label_values")
	assert.Contains(t, docs["prometheus"].Functions, "list_datasources")
}

func TestPluginGettingStartedSnippet(t *testing.T) {
	p := New()
	snippet := p.GettingStartedSnippet()
	assert.Equal(t, "", snippet)
}

func TestPluginRegisterResources(t *testing.T) {
	p := New()
	// RegisterResources is a no-op for Prometheus, should not error
	err := p.RegisterResources(nil, nil)
	assert.NoError(t, err)
}

func TestPluginStartStop(t *testing.T) {
	p := New()

	ctx := t.Context()

	// Start and Stop are no-ops for Prometheus
	err := p.Start(ctx)
	assert.NoError(t, err)

	err = p.Stop(ctx)
	assert.NoError(t, err)
}

// Additional tests for config methods
func TestConfigValidateDirect(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		expectError bool
	}{
		{
			name: "valid config",
			config: Config{
				Instances: []InstanceConfig{
					{Name: "prom1", URL: "http://localhost:9090"},
				},
			},
			expectError: false,
		},
		{
			name: "duplicate names",
			config: Config{
				Instances: []InstanceConfig{
					{Name: "same", URL: "http://localhost:9090"},
					{Name: "same", URL: "http://localhost:9091"},
				},
			},
			expectError: true,
		},
		{
			name: "missing URL",
			config: Config{
				Instances: []InstanceConfig{
					{Name: "prom1"},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfigApplyDefaultsDirect(t *testing.T) {
	cfg := Config{
		Instances: []InstanceConfig{
			{Name: "prom1", URL: "http://localhost:9090"},
			{Name: "prom2", URL: "http://localhost:9091", Timeout: 120},
		},
	}

	cfg.ApplyDefaults()

	// First instance should get default timeout
	assert.Equal(t, 60, cfg.Instances[0].Timeout)
	// Second instance should keep its custom timeout
	assert.Equal(t, 120, cfg.Instances[1].Timeout)
}
