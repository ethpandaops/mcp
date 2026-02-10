package syncoor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/ethpandaops/mcp/pkg/plugin"
	"github.com/ethpandaops/mcp/pkg/resource"
	"github.com/ethpandaops/mcp/pkg/types"
)

// Plugin implements the plugin.Plugin interface for Syncoor sync test monitoring.
type Plugin struct {
	cfg                 Config
	cartographoorClient resource.CartographoorClient
	log                 logrus.FieldLogger
}

// New creates a new Syncoor plugin.
func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string { return "syncoor" }

// DefaultEnabled implements plugin.DefaultEnabled.
// Syncoor is enabled by default since it requires no configuration.
func (p *Plugin) DefaultEnabled() bool { return true }

func (p *Plugin) Init(rawConfig []byte) error {
	if len(rawConfig) == 0 {
		// No config provided, use defaults (enabled = true).
		return nil
	}

	return yaml.Unmarshal(rawConfig, &p.cfg)
}

func (p *Plugin) ApplyDefaults() {
	// Defaults are handled by Config.IsEnabled().
}

func (p *Plugin) Validate() error {
	// No validation needed - config is minimal.
	return nil
}

// SandboxEnv returns environment variables for the sandbox.
// Returns ETHPANDAOPS_SYNCOOR_NETWORKS with network->URL mapping from cartographoor.
func (p *Plugin) SandboxEnv() (map[string]string, error) {
	if !p.cfg.IsEnabled() {
		return nil, nil
	}

	if p.cartographoorClient == nil {
		// Cartographoor client not yet set - return empty.
		// This will be populated after SetCartographoorClient is called.
		return nil, nil
	}

	// Build network -> Syncoor URL mapping from cartographoor data.
	networks := p.cartographoorClient.GetActiveNetworks()
	syncoorNetworks := make(map[string]string, len(networks))

	for name, network := range networks {
		if network.ServiceURLs != nil && network.ServiceURLs.Syncoor != "" {
			syncoorNetworks[name] = network.ServiceURLs.Syncoor
		}
	}

	if len(syncoorNetworks) == 0 {
		return nil, nil
	}

	networksJSON, err := json.Marshal(syncoorNetworks)
	if err != nil {
		return nil, fmt.Errorf("marshaling syncoor networks: %w", err)
	}

	return map[string]string{
		"ETHPANDAOPS_SYNCOOR_NETWORKS": string(networksJSON),
	}, nil
}

// DatasourceInfo returns empty since networks are the datasources,
// and those come from cartographoor.
func (p *Plugin) DatasourceInfo() []types.DatasourceInfo {
	return nil
}

func (p *Plugin) Examples() map[string]types.ExampleCategory {
	if !p.cfg.IsEnabled() {
		return nil
	}

	result := make(map[string]types.ExampleCategory, len(queryExamples))
	for k, v := range queryExamples {
		result[k] = v
	}

	return result
}

func (p *Plugin) PythonAPIDocs() map[string]types.ModuleDoc {
	if !p.cfg.IsEnabled() {
		return nil
	}

	return map[string]types.ModuleDoc{
		"syncoor": {
			Description: "Query Syncoor sync test orchestration and monitoring tool",
			Functions: map[string]types.FunctionDoc{
				"list_networks":          {Signature: "list_networks() -> list[dict]", Description: "List networks with Syncoor instances"},
				"get_base_url":           {Signature: "get_base_url(network) -> str", Description: "Get Syncoor base URL for a network"},
				"list_tests":             {Signature: "list_tests(network, active_only=False) -> list[dict]", Description: "List sync tests"},
				"get_test":               {Signature: "get_test(network, run_id) -> dict", Description: "Get detailed test information"},
				"get_test_progress":      {Signature: "get_test_progress(network, run_id) -> dict", Description: "Get test progress and current metrics"},
				"get_client_statistics":  {Signature: "get_client_statistics(network, run_id) -> dict", Description: "Get client sync statistics"},
				"link_test":              {Signature: "link_test(network, run_id) -> str", Description: "Deep link to test page"},
				"link_tests_list":        {Signature: "link_tests_list(network) -> str", Description: "Deep link to tests list page"},
			},
		},
	}
}

func (p *Plugin) GettingStartedSnippet() string {
	if !p.cfg.IsEnabled() {
		return ""
	}

	return `## Syncoor Sync Test Monitoring

Query the Syncoor sync test orchestration and monitoring tool for Ethereum client synchronization tests.
Generate deep links to view test details in the Syncoor web UI.

` + "```python" + `
from ethpandaops import syncoor

# List networks with Syncoor instances
networks = syncoor.list_networks()

# List all sync tests for a network
tests = syncoor.list_tests("mainnet")

# Get detailed information about a specific test
test = syncoor.get_test("mainnet", "test-run-id-123")
print(f"Test status: {test['status']}")

# Get test progress metrics
progress = syncoor.get_test_progress("mainnet", "test-run-id-123")
print(f"Sync progress: {progress['sync_percent']}%")

# Generate a deep link to view the test
link = syncoor.link_test("mainnet", "test-run-id-123")
print(f"View in Syncoor: {link}")
` + "```" + `
`
}

// SetCartographoorClient implements plugin.CartographoorAware.
// This is called by the builder to inject the cartographoor client.
func (p *Plugin) SetCartographoorClient(client any) {
	if c, ok := client.(resource.CartographoorClient); ok {
		p.cartographoorClient = c
	}
}

// SetLogger sets the logger for the plugin.
func (p *Plugin) SetLogger(log logrus.FieldLogger) {
	p.log = log.WithField("plugin", "syncoor")
}

// RegisterResources is a no-op since Syncoor uses networks:// resources.
func (p *Plugin) RegisterResources(_ logrus.FieldLogger, _ plugin.ResourceRegistry) error {
	return nil
}

func (p *Plugin) Start(_ context.Context) error { return nil }

func (p *Plugin) Stop(_ context.Context) error { return nil }
