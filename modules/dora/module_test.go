package dora

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ethpandaops/cartographoor/pkg/discovery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ethpandaops/panda/pkg/module"
)

type stubCartographoorClient struct {
	active map[string]discovery.Network
}

func (s stubCartographoorClient) Start(context.Context) error                     { return nil }
func (s stubCartographoorClient) Stop() error                                     { return nil }
func (s stubCartographoorClient) GetAllNetworks() map[string]discovery.Network    { return s.active }
func (s stubCartographoorClient) GetActiveNetworks() map[string]discovery.Network { return s.active }
func (s stubCartographoorClient) GetNetwork(string) (discovery.Network, bool) {
	return discovery.Network{}, false
}
func (s stubCartographoorClient) GetGroup(string) (map[string]discovery.Network, bool) {
	return nil, false
}
func (s stubCartographoorClient) GetGroups() []string                    { return nil }
func (s stubCartographoorClient) IsDevnet(discovery.Network) bool        { return false }
func (s stubCartographoorClient) GetClusters(discovery.Network) []string { return nil }

func TestConfigIsEnabledDefaultsToTrue(t *testing.T) {
	t.Parallel()

	var cfg Config
	assert.True(t, cfg.IsEnabled())

	disabled := false
	cfg.Enabled = &disabled
	assert.False(t, cfg.IsEnabled())
}

func TestModuleValidateLoadsExamplesAndExamplesReturnsClone(t *testing.T) {
	t.Parallel()

	mod := New()
	require.NoError(t, mod.Init(nil))
	require.NoError(t, mod.Validate())

	examples := mod.Examples()
	require.NotEmpty(t, examples)

	for key := range examples {
		delete(examples, key)
		break
	}

	assert.NotEmpty(t, mod.Examples())
}

func TestModuleSandboxEnvUsesActiveNetworksWithDoraURLs(t *testing.T) {
	t.Parallel()

	mod := New()
	mod.BindRuntimeDependencies(module.RuntimeDependencies{
		Cartographoor: stubCartographoorClient{
			active: map[string]discovery.Network{
				"hoodi": {
					Status: "active",
					ServiceURLs: &discovery.ServiceURLs{
						Dora: "https://hoodi.example",
					},
				},
				"ephemery": {
					Status:      "active",
					ServiceURLs: &discovery.ServiceURLs{},
				},
			},
		},
	})

	env, err := mod.SandboxEnv()
	require.NoError(t, err)
	require.Contains(t, env, "ETHPANDAOPS_DORA_NETWORKS")

	var payload map[string]string
	require.NoError(t, json.Unmarshal([]byte(env["ETHPANDAOPS_DORA_NETWORKS"]), &payload))
	assert.Equal(t, map[string]string{"hoodi": "https://hoodi.example"}, payload)
}

func TestModuleSandboxEnvReturnsNilWhenDisabled(t *testing.T) {
	t.Parallel()

	mod := New()
	require.NoError(t, mod.Init([]byte("enabled: false")))

	env, err := mod.SandboxEnv()
	require.NoError(t, err)
	assert.Nil(t, env)
	assert.Nil(t, mod.Examples())
	assert.Nil(t, mod.PythonAPIDocs())
	assert.Empty(t, mod.GettingStartedSnippet())
}
