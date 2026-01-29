package dora

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

// Plugin implements the plugin.Plugin interface for Dora beacon chain explorer.
type Plugin struct {
	cfg                 Config
	cartographoorClient resource.CartographoorClient
	log                 logrus.FieldLogger
}

// New creates a new Dora plugin.
func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string { return "dora" }

// DefaultEnabled implements plugin.DefaultEnabled.
// Dora is enabled by default since it requires no configuration.
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
// Returns ETHPANDAOPS_DORA_NETWORKS with network->URL mapping from cartographoor.
func (p *Plugin) SandboxEnv() (map[string]string, error) {
	if !p.cfg.IsEnabled() {
		return nil, nil
	}

	if p.cartographoorClient == nil {
		// Cartographoor client not yet set - return empty.
		// This will be populated after SetCartographoorClient is called.
		return nil, nil
	}

	// Build network -> Dora URL mapping from cartographoor data.
	networks := p.cartographoorClient.GetActiveNetworks()
	doraNetworks := make(map[string]string, len(networks))

	for name, network := range networks {
		if network.ServiceURLs != nil && network.ServiceURLs.Dora != "" {
			doraNetworks[name] = network.ServiceURLs.Dora
		}
	}

	if len(doraNetworks) == 0 {
		return nil, nil
	}

	networksJSON, err := json.Marshal(doraNetworks)
	if err != nil {
		return nil, fmt.Errorf("marshaling dora networks: %w", err)
	}

	return map[string]string{
		"ETHPANDAOPS_DORA_NETWORKS": string(networksJSON),
	}, nil
}

// ProxyConfig returns nil since Dora is public and needs no proxy.
func (p *Plugin) ProxyConfig() any {
	return nil
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
		"dora": {
			Description: "Query Dora beacon chain explorer and generate deep links",
			Functions: map[string]types.FunctionDoc{
				"list_networks": {
					Signature:   "dora.list_networks() -> list[dict]",
					Description: "List networks with Dora explorers. Uses networks discovered from cartographoor.",
					Returns:     "List of dicts with 'name', 'dora_url' keys",
				},
				"get_base_url": {
					Signature:   "dora.get_base_url(network: str) -> str",
					Description: "Get the Dora base URL for a network",
					Parameters: map[string]string{
						"network": "Network name (e.g., 'holesky', 'mainnet')",
					},
					Returns: "Dora base URL string",
				},
				"get_network_overview": {
					Signature:   "dora.get_network_overview(network: str) -> dict",
					Description: "Get network overview including current epoch, slot, and validator counts",
					Parameters: map[string]string{
						"network": "Network name",
					},
					Returns: "Dict with current_epoch, current_slot, active_validator_count, etc.",
				},
				"get_validator": {
					Signature:   "dora.get_validator(network: str, index_or_pubkey: str) -> dict",
					Description: "Get validator details by index or public key",
					Parameters: map[string]string{
						"network":         "Network name",
						"index_or_pubkey": "Validator index (e.g., '12345') or public key",
					},
					Returns: "Dict with status, balance, activation_epoch, etc.",
				},
				"get_validators": {
					Signature:   "dora.get_validators(network: str, status: str = None, limit: int = 100) -> list",
					Description: "Get list of validators with optional status filter",
					Parameters: map[string]string{
						"network": "Network name",
						"status":  "Filter by status: 'active', 'pending', 'exited', etc.",
						"limit":   "Maximum number of validators to return",
					},
					Returns: "List of validator dicts",
				},
				"get_slot": {
					Signature:   "dora.get_slot(network: str, slot_or_hash: str) -> dict",
					Description: "Get slot details by slot number or block hash",
					Parameters: map[string]string{
						"network":      "Network name",
						"slot_or_hash": "Slot number or block hash",
					},
					Returns: "Dict with slot, proposer, status, etc.",
				},
				"get_epoch": {
					Signature:   "dora.get_epoch(network: str, epoch: int) -> dict",
					Description: "Get epoch summary",
					Parameters: map[string]string{
						"network": "Network name",
						"epoch":   "Epoch number",
					},
					Returns: "Dict with epoch stats and finalization status",
				},
				"link_validator": {
					Signature:   "dora.link_validator(network: str, index_or_pubkey: str) -> str",
					Description: "Generate a Dora deep link to a validator",
					Parameters: map[string]string{
						"network":         "Network name",
						"index_or_pubkey": "Validator index or public key",
					},
					Returns: "URL string to view validator in Dora",
				},
				"link_slot": {
					Signature:   "dora.link_slot(network: str, slot_or_hash: str) -> str",
					Description: "Generate a Dora deep link to a slot/block",
					Parameters: map[string]string{
						"network":      "Network name",
						"slot_or_hash": "Slot number or block hash",
					},
					Returns: "URL string to view slot in Dora",
				},
				"link_epoch": {
					Signature:   "dora.link_epoch(network: str, epoch: int) -> str",
					Description: "Generate a Dora deep link to an epoch",
					Parameters: map[string]string{
						"network": "Network name",
						"epoch":   "Epoch number",
					},
					Returns: "URL string to view epoch in Dora",
				},
				"link_address": {
					Signature:   "dora.link_address(network: str, address: str) -> str",
					Description: "Generate a Dora deep link to an execution layer address",
					Parameters: map[string]string{
						"network": "Network name",
						"address": "Ethereum address (0x...)",
					},
					Returns: "URL string to view address in Dora",
				},
				"link_block": {
					Signature:   "dora.link_block(network: str, number_or_hash: str) -> str",
					Description: "Generate a Dora deep link to an execution layer block",
					Parameters: map[string]string{
						"network":        "Network name",
						"number_or_hash": "Block number or hash",
					},
					Returns: "URL string to view block in Dora",
				},
			},
		},
	}
}

func (p *Plugin) GettingStartedSnippet() string {
	if !p.cfg.IsEnabled() {
		return ""
	}

	return `## Dora Beacon Chain Explorer

Query the Dora beacon chain explorer for network status, validators, and slots.
Generate deep links to view data in the Dora web UI.

` + "```python" + `
from ethpandaops import dora

# List networks with Dora explorers
networks = dora.list_networks()

# Get network overview
overview = dora.get_network_overview("holesky")
print(f"Current epoch: {overview['current_epoch']}")

# Look up a validator and get a deep link
validator = dora.get_validator("holesky", "12345")
link = dora.link_validator("holesky", "12345")
print(f"View in Dora: {link}")
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
	p.log = log.WithField("plugin", "dora")
}

// RegisterResources registers Dora-specific MCP resources.
func (p *Plugin) RegisterResources(log logrus.FieldLogger, reg plugin.ResourceRegistry) error {
	if !p.cfg.IsEnabled() {
		return nil
	}

	p.log = log.WithField("plugin", "dora")

	if p.cartographoorClient == nil {
		p.log.Warn("Cartographoor client not set, skipping Dora resources registration")

		return nil
	}

	RegisterDoraResources(p.log, reg, p.cartographoorClient)

	return nil
}

func (p *Plugin) Start(_ context.Context) error { return nil }

func (p *Plugin) Stop(_ context.Context) error { return nil }
