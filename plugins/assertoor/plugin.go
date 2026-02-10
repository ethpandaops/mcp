package assertoor

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

// Plugin implements the plugin.Plugin interface for Assertoor testnet testing tool.
type Plugin struct {
	cfg                 Config
	cartographoorClient resource.CartographoorClient
	log                 logrus.FieldLogger
}

// New creates a new Assertoor plugin.
func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string { return "assertoor" }

// DefaultEnabled implements plugin.DefaultEnabled.
// Assertoor is enabled by default since it requires no configuration.
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
// Returns ETHPANDAOPS_ASSERTOOR_NETWORKS with network->URL mapping from cartographoor.
func (p *Plugin) SandboxEnv() (map[string]string, error) {
	if !p.cfg.IsEnabled() {
		return nil, nil
	}

	if p.cartographoorClient == nil {
		// Cartographoor client not yet set - return empty.
		// This will be populated after SetCartographoorClient is called.
		return nil, nil
	}

	// Build network -> Assertoor URL mapping from cartographoor data.
	networks := p.cartographoorClient.GetActiveNetworks()
	assertoorNetworks := make(map[string]string, len(networks))

	for name, network := range networks {
		if network.ServiceURLs != nil && network.ServiceURLs.Assertoor != "" {
			assertoorNetworks[name] = network.ServiceURLs.Assertoor
		}
	}

	if len(assertoorNetworks) == 0 {
		return nil, nil
	}

	networksJSON, err := json.Marshal(assertoorNetworks)
	if err != nil {
		return nil, fmt.Errorf("marshaling assertoor networks: %w", err)
	}

	return map[string]string{
		"ETHPANDAOPS_ASSERTOOR_NETWORKS": string(networksJSON),
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
		"assertoor": {
			Description: "Access Assertoor testnet testing tool for Ethereum network tests",
			Functions: map[string]types.FunctionDoc{
				"list_networks":        {Signature: "list_networks() -> list[dict]", Description: "List networks with Assertoor instances"},
				"get_base_url":         {Signature: "get_base_url(network) -> str", Description: "Get Assertoor base URL for a network"},
				"get_tests":            {Signature: "get_tests(network) -> list[dict]", Description: "List available test definitions"},
				"get_test_runs":        {Signature: "get_test_runs(network, test_id=None, limit=100) -> list[dict]", Description: "List test runs with optional filter"},
				"get_test_run_details": {Signature: "get_test_run_details(network, run_id) -> dict", Description: "Get detailed test run with tasks"},
				"get_test_run_status":  {Signature: "get_test_run_status(network, run_id) -> dict", Description: "Get test run status summary"},
				"get_task_details":     {Signature: "get_task_details(network, run_id, task_index) -> dict", Description: "Get task details with logs"},
				"link_test_run":        {Signature: "link_test_run(network, run_id) -> str", Description: "Deep link to test run page"},
				"link_task":            {Signature: "link_task(network, run_id, task_index) -> str", Description: "Deep link to task page"},
				"link_task_logs":       {Signature: "link_task_logs(network, run_id, task_index) -> str", Description: "Deep link to task logs"},
			},
		},
	}
}

func (p *Plugin) GettingStartedSnippet() string {
	if !p.cfg.IsEnabled() {
		return ""
	}

	return `## Assertoor Testnet Testing

Access the Assertoor testnet testing tool to monitor test runs, check results,
and generate deep links to the web UI.

` + "```python" + `
from ethpandaops import assertoor

# List networks with Assertoor instances
networks = assertoor.list_networks()

# Get recent test runs
test_runs = assertoor.get_test_runs("holesky")

# Get details of a specific test run
details = assertoor.get_test_run_details("holesky", 123)
print(f"Test status: {details['status']}")

# Generate a link to view in Assertoor UI
link = assertoor.link_test_run("holesky", 123)
print(f"View in Assertoor: {link}")
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
	p.log = log.WithField("plugin", "assertoor")
}

// RegisterResources is a no-op since Assertoor uses networks:// resources.
func (p *Plugin) RegisterResources(_ logrus.FieldLogger, _ plugin.ResourceRegistry) error {
	return nil
}

func (p *Plugin) Start(_ context.Context) error { return nil }

func (p *Plugin) Stop(_ context.Context) error { return nil }
