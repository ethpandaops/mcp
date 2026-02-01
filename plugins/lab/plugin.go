package lab

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

// defaultLabURLs provides fallback Lab URLs for known networks.
var defaultLabURLs = map[string]string{
	"mainnet": "https://lab.ethpandaops.io",
	"sepolia": "https://lab.ethpandaops.io/sepolia",
	"holesky": "https://lab.ethpandaops.io/holesky",
	"hoodi":   "https://lab.ethpandaops.io/hoodi",
}

// Plugin implements the plugin.Plugin interface for Lab explorer.
type Plugin struct {
	cfg                 Config
	cartographoorClient resource.CartographoorClient
	log                 logrus.FieldLogger
}

// New creates a new Lab plugin.
func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string { return "lab" }

// DefaultEnabled implements plugin.DefaultEnabled.
// Lab is enabled by default since it requires no configuration.
func (p *Plugin) DefaultEnabled() bool { return true }

func (p *Plugin) Init(rawConfig []byte) error {
	if len(rawConfig) == 0 {
		// No config provided, use defaults (enabled = true).
		return nil
	}

	return yaml.Unmarshal(rawConfig, &p.cfg)
}

func (p *Plugin) ApplyDefaults() {
	// Defaults are handled by Config methods.
}

func (p *Plugin) Validate() error {
	// No validation needed - config is minimal.
	return nil
}

// SandboxEnv returns environment variables for the sandbox.
// Returns ETHPANDAOPS_LAB_NETWORKS with network->URL mapping.
func (p *Plugin) SandboxEnv() (map[string]string, error) {
	if !p.cfg.IsEnabled() {
		return nil, nil
	}

	// Build network -> Lab URL mapping from config or defaults.
	labNetworks := make(map[string]string)

	// First, add any custom networks from config
	if p.cfg.Networks != nil {
		for name, url := range p.cfg.Networks {
			labNetworks[name] = url
		}
	}

	// Then, add known networks from cartographoor if available
	if p.cartographoorClient != nil {
		networks := p.cartographoorClient.GetActiveNetworks()
		for name := range networks {
			if url, ok := defaultLabURLs[name]; ok {
				if _, alreadySet := labNetworks[name]; !alreadySet {
					labNetworks[name] = url
				}
			}
		}
	}

	// Ensure default networks are always available
	for name, url := range defaultLabURLs {
		if _, ok := labNetworks[name]; !ok {
			labNetworks[name] = url
		}
	}

	networksJSON, err := json.Marshal(labNetworks)
	if err != nil {
		return nil, fmt.Errorf("marshaling lab networks: %w", err)
	}

	// Also pass routes and skill URLs
	env := map[string]string{
		"ETHPANDAOPS_LAB_NETWORKS":   string(networksJSON),
		"ETHPANDAOPS_LAB_ROUTES_URL": p.cfg.GetRoutesURL(),
		"ETHPANDAOPS_LAB_SKILL_URL":  p.cfg.GetSkillURL(),
	}

	return env, nil
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
		"lab": {
			Description: "Query Lab explorer and generate deep links using routes.json and SKILL.md patterns",
			Functions: map[string]types.FunctionDoc{
				"list_networks":          {Signature: "list_networks() -> list[dict]", Description: "List networks with Lab explorers"},
				"get_base_url":           {Signature: "get_base_url(network) -> str", Description: "Get Lab base URL for a network"},
				"get_routes":             {Signature: "get_routes() -> dict", Description: "Get all Lab routes from routes.json"},
				"get_routes_by_category": {Signature: "get_routes_by_category(category) -> list[dict]", Description: "Get routes filtered by category"},
				"get_route":              {Signature: "get_route(route_id) -> dict", Description: "Get route metadata by ID"},
				"link_slot":              {Signature: "link_slot(network, slot) -> str", Description: "Deep link to slot"},
				"link_epoch":             {Signature: "link_epoch(network, epoch) -> str", Description: "Deep link to epoch"},
				"link_validator":         {Signature: "link_validator(network, index_or_pubkey) -> str", Description: "Deep link to validator"},
				"link_block":             {Signature: "link_block(network, number_or_hash) -> str", Description: "Deep link to block"},
				"link_address":           {Signature: "link_address(network, address) -> str", Description: "Deep link to address"},
				"link_transaction":       {Signature: "link_transaction(network, tx_hash) -> str", Description: "Deep link to transaction"},
				"link_blob":              {Signature: "link_blob(network, blob_id) -> str", Description: "Deep link to blob"},
				"link_fork":              {Signature: "link_fork(network, fork_name) -> str", Description: "Deep link to fork"},
				"build_url":              {Signature: "build_url(network, route_id, params) -> str", Description: "Build URL for route with parameters"},
			},
		},
	}
}

func (p *Plugin) GettingStartedSnippet() string {
	if !p.cfg.IsEnabled() {
		return ""
	}

	return `## Lab Explorer

Query the Lab explorer for Ethereum network data and generate deep links.
Lab uses routes.json for URL patterns and SKILL.md for deep linking.

` + "```python" + `
from ethpandaops import lab

# List networks with Lab explorers
networks = lab.list_networks()

# Get available routes
routes = lab.get_routes()
print(f"Available categories: {list(routes.keys())}")

# Generate deep links
link = lab.link_slot("mainnet", 1000000)
print(f"View slot: {link}")

# Build custom URLs
url = lab.build_url("sepolia", "epoch", {"epoch": 5000})
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
	p.log = log.WithField("plugin", "lab")
}

// RegisterResources is a no-op since Lab uses networks:// resources.
func (p *Plugin) RegisterResources(_ logrus.FieldLogger, _ plugin.ResourceRegistry) error {
	return nil
}

func (p *Plugin) Start(_ context.Context) error { return nil }

func (p *Plugin) Stop(_ context.Context) error { return nil }
