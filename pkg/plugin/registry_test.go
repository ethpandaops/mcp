package plugin

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ethpandaops/mcp/pkg/types"
)

// mockPlugin is a test implementation of the Plugin interface
type mockPlugin struct {
	name          string
	initCalled    bool
	validateErr   error
	sandboxEnv    map[string]string
	datasourceInfo []types.DatasourceInfo
	examples      map[string]types.ExampleCategory
	apiDocs       map[string]types.ModuleDoc
	started       bool
	stopped       bool
}

func (m *mockPlugin) Name() string                              { return m.name }
func (m *mockPlugin) Init(rawConfig []byte) error               { m.initCalled = true; return nil }
func (m *mockPlugin) ApplyDefaults()                            {}
func (m *mockPlugin) Validate() error                           { return m.validateErr }
func (m *mockPlugin) SandboxEnv() (map[string]string, error)    { return m.sandboxEnv, nil }
func (m *mockPlugin) DatasourceInfo() []types.DatasourceInfo    { return m.datasourceInfo }
func (m *mockPlugin) Examples() map[string]types.ExampleCategory { return m.examples }
func (m *mockPlugin) PythonAPIDocs() map[string]types.ModuleDoc { return m.apiDocs }
func (m *mockPlugin) GettingStartedSnippet() string             { return "" }
func (m *mockPlugin) RegisterResources(log logrus.FieldLogger, reg ResourceRegistry) error { return nil }
func (m *mockPlugin) Start(ctx context.Context) error           { m.started = true; return nil }
func (m *mockPlugin) Stop(ctx context.Context) error            { m.stopped = true; return nil }

func TestNewRegistry(t *testing.T) {
	log := logrus.New()
	reg := NewRegistry(log)

	assert.NotNil(t, reg)
	assert.NotNil(t, reg.all)
	assert.Empty(t, reg.initialized)
}

func TestRegistryAdd(t *testing.T) {
	log := logrus.New()
	reg := NewRegistry(log)

	plugin1 := &mockPlugin{name: "plugin1"}
	plugin2 := &mockPlugin{name: "plugin2"}

	reg.Add(plugin1)
	reg.Add(plugin2)

	assert.Equal(t, 2, len(reg.All()))
	assert.Contains(t, reg.All(), "plugin1")
	assert.Contains(t, reg.All(), "plugin2")
}

func TestRegistryGet(t *testing.T) {
	log := logrus.New()
	reg := NewRegistry(log)

	plugin := &mockPlugin{name: "test-plugin"}
	reg.Add(plugin)

	// Test getting existing plugin
	p := reg.Get("test-plugin")
	assert.Equal(t, plugin, p)

	// Test getting non-existent plugin
	p = reg.Get("nonexistent")
	assert.Nil(t, p)
}

func TestRegistryInitPlugin(t *testing.T) {
	log := logrus.New()
	reg := NewRegistry(log)

	plugin := &mockPlugin{name: "test-plugin"}
	reg.Add(plugin)

	err := reg.InitPlugin("test-plugin", []byte("config: value"))
	require.NoError(t, err)
	assert.True(t, plugin.initCalled)
	assert.Equal(t, 1, len(reg.Initialized()))
}

func TestRegistryInitPluginUnknown(t *testing.T) {
	log := logrus.New()
	reg := NewRegistry(log)

	err := reg.InitPlugin("unknown-plugin", []byte("config"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown plugin")
}

func TestRegistryInitPluginValidationError(t *testing.T) {
	log := logrus.New()
	reg := NewRegistry(log)

	plugin := &mockPlugin{
		name:        "test-plugin",
		validateErr: assert.AnError,
	}
	reg.Add(plugin)

	err := reg.InitPlugin("test-plugin", []byte("config"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "validating")
}

func TestRegistryStartAll(t *testing.T) {
	log := logrus.New()
	reg := NewRegistry(log)

	plugin1 := &mockPlugin{name: "plugin1"}
	plugin2 := &mockPlugin{name: "plugin2"}

	reg.Add(plugin1)
	reg.Add(plugin2)

	err := reg.InitPlugin("plugin1", []byte("config"))
	require.NoError(t, err)
	err = reg.InitPlugin("plugin2", []byte("config"))
	require.NoError(t, err)

	ctx := context.Background()
	err = reg.StartAll(ctx)
	require.NoError(t, err)

	assert.True(t, plugin1.started)
	assert.True(t, plugin2.started)
}

func TestRegistryStopAll(t *testing.T) {
	log := logrus.New()
	reg := NewRegistry(log)

	plugin1 := &mockPlugin{name: "plugin1"}
	plugin2 := &mockPlugin{name: "plugin2"}

	reg.Add(plugin1)
	reg.Add(plugin2)

	_ = reg.InitPlugin("plugin1", []byte("config"))
	_ = reg.InitPlugin("plugin2", []byte("config"))

	ctx := context.Background()
	reg.StopAll(ctx)

	assert.True(t, plugin1.stopped)
	assert.True(t, plugin2.stopped)
}

func TestRegistrySandboxEnv(t *testing.T) {
	log := logrus.New()
	reg := NewRegistry(log)

	plugin1 := &mockPlugin{
		name: "plugin1",
		sandboxEnv: map[string]string{
			"VAR1": "value1",
		},
	}
	plugin2 := &mockPlugin{
		name: "plugin2",
		sandboxEnv: map[string]string{
			"VAR2": "value2",
		},
	}

	reg.Add(plugin1)
	reg.Add(plugin2)

	_ = reg.InitPlugin("plugin1", []byte("config"))
	_ = reg.InitPlugin("plugin2", []byte("config"))

	env, err := reg.SandboxEnv()
	require.NoError(t, err)

	assert.Equal(t, "value1", env["VAR1"])
	assert.Equal(t, "value2", env["VAR2"])
}

func TestRegistryDatasourceInfo(t *testing.T) {
	log := logrus.New()
	reg := NewRegistry(log)

	plugin1 := &mockPlugin{
		name: "plugin1",
		datasourceInfo: []types.DatasourceInfo{
			{Type: "type1", Name: "ds1"},
		},
	}
	plugin2 := &mockPlugin{
		name: "plugin2",
		datasourceInfo: []types.DatasourceInfo{
			{Type: "type2", Name: "ds2"},
			{Type: "type3", Name: "ds3"},
		},
	}

	reg.Add(plugin1)
	reg.Add(plugin2)

	_ = reg.InitPlugin("plugin1", []byte("config"))
	_ = reg.InitPlugin("plugin2", []byte("config"))

	infos := reg.DatasourceInfo()
	assert.Equal(t, 3, len(infos))
}

func TestRegistryExamples(t *testing.T) {
	log := logrus.New()
	reg := NewRegistry(log)

	plugin1 := &mockPlugin{
		name: "plugin1",
		examples: map[string]types.ExampleCategory{
			"cat1": {Name: "Category 1"},
		},
	}
	plugin2 := &mockPlugin{
		name: "plugin2",
		examples: map[string]types.ExampleCategory{
			"cat2": {Name: "Category 2"},
		},
	}

	reg.Add(plugin1)
	reg.Add(plugin2)

	_ = reg.InitPlugin("plugin1", []byte("config"))
	_ = reg.InitPlugin("plugin2", []byte("config"))

	examples := reg.Examples()
	assert.Equal(t, 2, len(examples))
	assert.Contains(t, examples, "cat1")
	assert.Contains(t, examples, "cat2")
}

func TestRegistryAllExamples(t *testing.T) {
	log := logrus.New()
	reg := NewRegistry(log)

	plugin := &mockPlugin{
		name: "plugin1",
		examples: map[string]types.ExampleCategory{
			"cat1": {Name: "Category 1"},
		},
	}

	reg.Add(plugin)
	// Don't initialize - AllExamples should still work

	examples := reg.AllExamples()
	assert.Equal(t, 1, len(examples))
	assert.Contains(t, examples, "cat1")
}

func TestRegistryPythonAPIDocs(t *testing.T) {
	log := logrus.New()
	reg := NewRegistry(log)

	plugin1 := &mockPlugin{
		name: "plugin1",
		apiDocs: map[string]types.ModuleDoc{
			"module1": {Description: "Module 1"},
		},
	}

	reg.Add(plugin1)
	_ = reg.InitPlugin("plugin1", []byte("config"))

	docs := reg.PythonAPIDocs()
	assert.Equal(t, 1, len(docs))
	assert.Contains(t, docs, "module1")
}

func TestRegistryAllPythonAPIDocs(t *testing.T) {
	log := logrus.New()
	reg := NewRegistry(log)

	plugin := &mockPlugin{
		name: "plugin1",
		apiDocs: map[string]types.ModuleDoc{
			"module1": {Description: "Module 1"},
		},
	}

	reg.Add(plugin)
	// Don't initialize - AllPythonAPIDocs should still work

	docs := reg.AllPythonAPIDocs()
	assert.Equal(t, 1, len(docs))
	assert.Contains(t, docs, "module1")
}

func TestRegistryGettingStartedSnippets(t *testing.T) {
	log := logrus.New()
	reg := NewRegistry(log)

	// Note: mockPlugin.GettingStartedSnippet returns empty string
	plugin := &mockPlugin{name: "plugin1"}

	reg.Add(plugin)
	_ = reg.InitPlugin("plugin1", []byte("config"))

	snippets := reg.GettingStartedSnippets()
	assert.Equal(t, "", snippets)
}

func TestErrNoValidConfig(t *testing.T) {
	assert.NotNil(t, ErrNoValidConfig)
	assert.Equal(t, "no valid configuration entries", ErrNoValidConfig.Error())
}
