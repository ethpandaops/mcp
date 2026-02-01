package clickhouse

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	p := New()
	require.NotNil(t, p)
	assert.Equal(t, "clickhouse", p.Name())
}

func TestPluginInit(t *testing.T) {
	tests := []struct {
		name        string
		config      string
		expectError bool
	}{
		{
			name: "valid config with clusters",
			config: `
clusters:
  - name: test-cluster
    host: localhost
    database: testdb
    username: user
    password: pass
`,
			expectError: false,
		},
		{
			name: "valid config with schema discovery",
			config: `
schema_discovery:
  datasources:
    - name: ds1
      cluster: cluster1
`,
			expectError: false,
		},
		{
			name: "config filters unnamed clusters",
			config: `
clusters:
  - name: valid-cluster
    host: localhost
  - name: ""
    host: localhost
`,
			expectError: false,
		},
		{
			name:        "empty config",
			config:      "",
			expectError: false,
		},
		{
			name: "invalid yaml",
			config: `
clusters:
  - name: test
    host: [invalid
`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New()
			err := p.Init([]byte(tt.config))
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPluginApplyDefaults(t *testing.T) {
	p := New()

	err := p.Init([]byte(`
clusters:
  - name: test
    host: localhost
`))
	require.NoError(t, err)

	p.ApplyDefaults()

	assert.Equal(t, 120, p.cfg.Clusters[0].Timeout)
	assert.Equal(t, 15*time.Minute, p.cfg.SchemaDiscovery.RefreshInterval)
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
clusters:
  - name: cluster1
    host: localhost
    database: db1
`,
			expectError: false,
		},
		{
			name: "missing cluster name gets filtered during Init",
			config: `
clusters:
  - host: localhost
`,
			expectError: false, // Init filters out unnamed clusters, leaving empty config which is valid
		},
		{
			name: "cluster with empty name in array gets filtered",
			config: `
clusters:
  - name: valid
    host: localhost
  - name: ""
    host: other
`,
			expectError: false, // Init filters out the empty-named cluster
		},
		{
			name: "duplicate cluster names",
			config: `
clusters:
  - name: same
    host: host1
  - name: same
    host: host2
`,
			expectError: true,
			errorMsg:    "duplicated",
		},
		{
			name: "missing schema discovery datasource name gets filtered",
			config: `
schema_discovery:
  datasources:
    - cluster: cluster1
`,
			expectError: false, // Init filters out entries without names
		},
		{
			name:        "empty config is valid",
			config:      "",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New()
			err := p.Init([]byte(tt.config))
			require.NoError(t, err)

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
			name: "with clusters",
			config: `
clusters:
  - name: test-cluster
    host: localhost
    database: testdb
    description: Test cluster
`,
			expectedKeys: 1,
		},
		{
			name:         "no clusters",
			config:       "",
			expectedKeys: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New()
			err := p.Init([]byte(tt.config))
			require.NoError(t, err)

			env, err := p.SandboxEnv()
			require.NoError(t, err)
			assert.Equal(t, tt.expectedKeys, len(env))

			if tt.expectedKeys > 0 {
				assert.Contains(t, env, "ETHPANDAOPS_CLICKHOUSE_DATASOURCES")
			}
		})
	}
}

func TestPluginDatasourceInfo(t *testing.T) {
	p := New()
	err := p.Init([]byte(`
clusters:
  - name: cluster1
    host: host1
    database: db1
    description: First cluster
  - name: cluster2
    host: host2
    database: db2
    description: Second cluster
`))
	require.NoError(t, err)

	infos := p.DatasourceInfo()
	assert.Equal(t, 2, len(infos))
	assert.Equal(t, "clickhouse", infos[0].Type)
	assert.Equal(t, "cluster1", infos[0].Name)
	assert.Equal(t, "db1", infos[0].Metadata["database"])
}

func TestPluginExamples(t *testing.T) {
	p := New()
	examples := p.Examples()
	assert.NotEmpty(t, examples)
}

func TestPluginPythonAPIDocs(t *testing.T) {
	p := New()
	docs := p.PythonAPIDocs()
	assert.Contains(t, docs, "clickhouse")
	assert.NotEmpty(t, docs["clickhouse"].Description)
	assert.NotEmpty(t, docs["clickhouse"].Functions)
}

func TestPluginGettingStartedSnippet(t *testing.T) {
	p := New()
	snippet := p.GettingStartedSnippet()
	assert.Contains(t, snippet, "ClickHouse")
	assert.Contains(t, snippet, "xatu")
	assert.Contains(t, snippet, "xatu-cbt")
}

func TestPluginStartStop(t *testing.T) {
	p := New()

	// Init with disabled schema discovery
	err := p.Init([]byte(`
schema_discovery:
  enabled: false
`))
	require.NoError(t, err)

	ctx := t.Context()
	err = p.Start(ctx)
	// Should not error with schema discovery disabled
	assert.NoError(t, err)

	err = p.Stop(ctx)
	assert.NoError(t, err)
}

func TestClusterConfigIsSecure(t *testing.T) {
	tests := []struct {
		name     string
		secure   *bool
		expected bool
	}{
		{
			name:     "nil defaults to true",
			secure:   nil,
			expected: true,
		},
		{
			name:     "explicitly true",
			secure:   boolPtr(true),
			expected: true,
		},
		{
			name:     "explicitly false",
			secure:   boolPtr(false),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ClusterConfig{Secure: tt.secure}
			assert.Equal(t, tt.expected, cfg.IsSecure())
		})
	}
}

func TestSchemaDiscoveryConfigIsEnabled(t *testing.T) {
	tests := []struct {
		name        string
		enabled     *bool
		datasources int
		expected    bool
	}{
		{
			name:        "enabled nil with datasources defaults to true",
			enabled:     nil,
			datasources: 1,
			expected:    true,
		},
		{
			name:        "enabled nil without datasources defaults to false",
			enabled:     nil,
			datasources: 0,
			expected:    false,
		},
		{
			name:        "explicitly true",
			enabled:     boolPtr(true),
			datasources: 0,
			expected:    true,
		},
		{
			name:        "explicitly false",
			enabled:     boolPtr(false),
			datasources: 1,
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := SchemaDiscoveryConfig{
				Enabled:     tt.enabled,
				Datasources: make([]SchemaDiscoveryDatasource, tt.datasources),
			}
			assert.Equal(t, tt.expected, cfg.IsEnabled())
		})
	}
}

// Helper function
func boolPtr(b bool) *bool {
	return &b
}
