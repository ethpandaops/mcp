package loki

import (
	"testing"

	"github.com/ethpandaops/mcp/pkg/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	p := New()
	require.NotNil(t, p)
	assert.Equal(t, "loki", p.Name())
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
  - name: test-loki
    url: http://localhost:3100
`,
			expectError: false,
			expectSkip:  false,
		},
		{
			name: "config filters invalid instances keeps valid ones",
			config: `
instances:
  - name: valid
    url: http://localhost:3100
  - name: ""
    url: http://localhost:3101
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
    url: http://localhost:3100
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
    url: http://localhost:3100
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
  - name: loki1
    url: http://localhost:3100
`,
			expectError: false,
		},
		{
			name: "multiple valid instances",
			config: `
instances:
  - name: loki1
    url: http://localhost:3100
  - name: loki2
    url: http://localhost:3101
`,
			expectError: false,
		},
		{
			name: "missing instance name",
			config: `
instances:
  - url: http://localhost:3100
`,
			expectError: true,
			errorMsg:    "instances[0].name is required",
		},
		{
			name: "duplicate instance names",
			config: `
instances:
  - name: same
    url: http://localhost:3100
  - name: same
    url: http://localhost:3101
`,
			expectError: true,
			errorMsg:    "duplicated",
		},
		{
			name: "missing instance URL",
			config: `
instances:
  - name: loki1
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
  - name: loki1
    url: http://localhost:3100
    description: Test Loki
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
				assert.Contains(t, env, "ETHPANDAOPS_LOKI_DATASOURCES")
			}
		})
	}
}

func TestPluginDatasourceInfo(t *testing.T) {
	p := New()
	err := p.Init([]byte(`
instances:
  - name: loki1
    url: http://localhost:3100
    description: First Loki
  - name: loki2
    url: http://localhost:3101
    description: Second Loki
`))
	require.NoError(t, err)

	infos := p.DatasourceInfo()
	assert.Equal(t, 2, len(infos))
	assert.Equal(t, "loki", infos[0].Type)
	assert.Equal(t, "loki1", infos[0].Name)
	assert.Equal(t, "http://localhost:3100", infos[0].Metadata["url"])
}

func TestPluginExamples(t *testing.T) {
	p := New()
	examples := p.Examples()
	assert.NotEmpty(t, examples)
}

func TestPluginPythonAPIDocs(t *testing.T) {
	p := New()
	docs := p.PythonAPIDocs()
	assert.Contains(t, docs, "loki")
	assert.NotEmpty(t, docs["loki"].Description)
	assert.NotEmpty(t, docs["loki"].Functions)

	// Check specific functions
	assert.Contains(t, docs["loki"].Functions, "query")
	assert.Contains(t, docs["loki"].Functions, "query_instant")
	assert.Contains(t, docs["loki"].Functions, "get_labels")
	assert.Contains(t, docs["loki"].Functions, "get_label_values")
	assert.Contains(t, docs["loki"].Functions, "list_datasources")
}

func TestPluginGettingStartedSnippet(t *testing.T) {
	p := New()
	snippet := p.GettingStartedSnippet()
	assert.Equal(t, "", snippet)
}

func TestPluginRegisterResources(t *testing.T) {
	p := New()
	// RegisterResources is a no-op for Loki, should not error
	err := p.RegisterResources(nil, nil)
	assert.NoError(t, err)
}

func TestPluginStartStop(t *testing.T) {
	p := New()

	ctx := t.Context()

	// Start and Stop are no-ops for Loki
	err := p.Start(ctx)
	assert.NoError(t, err)

	err = p.Stop(ctx)
	assert.NoError(t, err)
}
