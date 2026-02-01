//go:build !skip_dora
// +build !skip_dora

package dora

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	p := New()
	require.NotNil(t, p)
	assert.Equal(t, "dora", p.Name())
}

func TestPluginDefaultEnabled(t *testing.T) {
	p := New()
	// Dora plugin implements DefaultEnabled interface
	assert.True(t, p.DefaultEnabled())
}

func TestPluginInit(t *testing.T) {
	tests := []struct {
		name        string
		config      string
		expectError bool
	}{
		{
			name: "explicitly enabled",
			config: `
enabled: true
`,
			expectError: false,
		},
		{
			name: "explicitly disabled",
			config: `
enabled: false
`,
			expectError: false,
		},
		{
			name:        "empty config uses defaults",
			config:      "",
			expectError: false,
		},
		{
			name:        "invalid yaml",
			config:      `enabled: [invalid`,
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

	// Init with empty config
	err := p.Init([]byte(""))
	require.NoError(t, err)

	// ApplyDefaults is a no-op for Dora
	p.ApplyDefaults()

	// Config should still use defaults (enabled = true)
	assert.True(t, p.cfg.IsEnabled())
}

func TestPluginValidate(t *testing.T) {
	p := New()

	// Validate is a no-op for Dora - always returns nil
	err := p.Validate()
	assert.NoError(t, err)
}

func TestPluginSandboxEnv(t *testing.T) {
	tests := []struct {
		name         string
		config       string
		expectedNil  bool
	}{
		{
			name:         "disabled returns nil",
			config:       `enabled: false`,
			expectedNil:  true,
		},
		{
			name:         "enabled but no cartographoor client",
			config:       `enabled: true`,
			expectedNil:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New()
			err := p.Init([]byte(tt.config))
			require.NoError(t, err)

			env, err := p.SandboxEnv()
			require.NoError(t, err)

			if tt.expectedNil {
				assert.Nil(t, env)
			}
		})
	}
}

func TestPluginDatasourceInfo(t *testing.T) {
	p := New()
	// Dora plugin returns nil for DatasourceInfo
	infos := p.DatasourceInfo()
	assert.Nil(t, infos)
}

func TestPluginExamples(t *testing.T) {
	tests := []struct {
		name     string
		config   string
		expected int
	}{
		{
			name:     "enabled returns examples",
			config:   `enabled: true`,
			expected: 0, // Will be > 0 if examples exist
		},
		{
			name:     "disabled returns nil",
			config:   `enabled: false`,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New()
			err := p.Init([]byte(tt.config))
			require.NoError(t, err)

			examples := p.Examples()
			if tt.config == "enabled: false" {
				assert.Nil(t, examples)
			} else {
				// Examples may or may not be empty, just ensure it doesn't panic
				_ = examples
			}
		})
	}
}

func TestPluginPythonAPIDocs(t *testing.T) {
	tests := []struct {
		name         string
		config       string
		shouldBeNil  bool
	}{
		{
			name:        "enabled returns docs",
			config:      `enabled: true`,
			shouldBeNil: false,
		},
		{
			name:        "disabled returns nil",
			config:      `enabled: false`,
			shouldBeNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New()
			err := p.Init([]byte(tt.config))
			require.NoError(t, err)

			docs := p.PythonAPIDocs()
			if tt.shouldBeNil {
				assert.Nil(t, docs)
			} else {
				assert.NotNil(t, docs)
				assert.Contains(t, docs, "dora")
				assert.NotEmpty(t, docs["dora"].Description)
				assert.NotEmpty(t, docs["dora"].Functions)
			}
		})
	}
}

func TestPluginGettingStartedSnippet(t *testing.T) {
	tests := []struct {
		name           string
		config         string
		shouldBeEmpty  bool
	}{
		{
			name:          "enabled returns snippet",
			config:        `enabled: true`,
			shouldBeEmpty: false,
		},
		{
			name:          "disabled returns empty",
			config:        `enabled: false`,
			shouldBeEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New()
			err := p.Init([]byte(tt.config))
			require.NoError(t, err)

			snippet := p.GettingStartedSnippet()
			if tt.shouldBeEmpty {
				assert.Equal(t, "", snippet)
			} else {
				assert.NotEmpty(t, snippet)
				assert.Contains(t, snippet, "Dora")
			}
		})
	}
}

func TestPluginRegisterResources(t *testing.T) {
	p := New()
	// RegisterResources is a no-op for Dora
	err := p.RegisterResources(nil, nil)
	assert.NoError(t, err)
}

func TestPluginStartStop(t *testing.T) {
	p := New()

	ctx := t.Context()

	// Start and Stop are no-ops for Dora
	err := p.Start(ctx)
	assert.NoError(t, err)

	err = p.Stop(ctx)
	assert.NoError(t, err)
}

func TestConfigIsEnabled(t *testing.T) {
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
			cfg := Config{Enabled: tt.enabled}
			assert.Equal(t, tt.expected, cfg.IsEnabled())
		})
	}
}

func TestPluginSetCartographoorClient(t *testing.T) {
	p := New()

	// Test setting nil client
	p.SetCartographoorClient(nil)
	assert.Nil(t, p.cartographoorClient)

	// Test setting non-client type
	p.SetCartographoorClient("not a client")
	assert.Nil(t, p.cartographoorClient)
}

func TestPluginSetLogger(t *testing.T) {
	p := New()

	// SetLogger should not panic
	p.SetLogger(nil)
	// We can't easily assert on the logger value, but we can verify no panic
}

// Helper function
func boolPtr(b bool) *bool {
	return &b
}
