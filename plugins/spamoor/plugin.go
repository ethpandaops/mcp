package spamoor

import (
	"context"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/ethpandaops/mcp/pkg/plugin"
	"github.com/ethpandaops/mcp/pkg/types"
)

// Plugin implements the plugin.Plugin interface for Spamoor transaction spammer.
type Plugin struct {
	cfg Config
	log logrus.FieldLogger
}

// New creates a new Spamoor plugin.
func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string { return "spamoor" }

// DefaultEnabled implements plugin.DefaultEnabled.
// Spamoor is disabled by default since it requires configuration.
func (p *Plugin) DefaultEnabled() bool { return false }

func (p *Plugin) Init(rawConfig []byte) error {
	if len(rawConfig) == 0 {
		// No config provided, use defaults (enabled = false).
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
// Returns ETHPANDAOPS_SPAMOOR_URL if configured.
func (p *Plugin) SandboxEnv() (map[string]string, error) {
	if !p.cfg.IsEnabled() {
		return nil, nil
	}

	return map[string]string{
		"ETHPANDAOPS_SPAMOOR_ENABLED": "true",
	}, nil
}

// DatasourceInfo returns empty since Spamoor doesn't use traditional datasources.
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
		"spamoor": {
			Description: "Interact with Spamoor transaction spamming daemon",
			Functions: map[string]types.FunctionDoc{
				"list_instances":    {Signature: "list_instances(base_url) -> list[dict]", Description: "List all spammer instances"},
				"get_instance":      {Signature: "get_instance(base_url, instance_id) -> dict", Description: "Get spammer instance details"},
				"start_instance":    {Signature: "start_instance(base_url, instance_id) -> dict", Description: "Start a spammer instance"},
				"stop_instance":     {Signature: "stop_instance(base_url, instance_id) -> dict", Description: "Stop (pause) a spammer instance"},
				"get_clients":       {Signature: "get_clients(base_url) -> list[dict]", Description: "List all RPC clients"},
				"get_wallets":       {Signature: "get_wallets(base_url) -> list[dict]", Description: "Get wallet information"},
				"get_pending_txs":   {Signature: "get_pending_txs(base_url, wallet: str | None = None) -> list[dict]", Description: "Get pending transactions"},
				"get_metrics":       {Signature: "get_metrics(base_url) -> dict", Description: "Get spammer metrics dashboard"},
				"link_instance":     {Signature: "link_instance(base_url, instance_id) -> str", Description: "Generate deep link to spammer"},
				"link_dashboard":    {Signature: "link_dashboard(base_url) -> str", Description: "Generate deep link to dashboard"},
				"link_wallets":      {Signature: "link_wallets(base_url) -> str", Description: "Generate deep link to wallets"},
			},
		},
	}
}

func (p *Plugin) GettingStartedSnippet() string {
	if !p.cfg.IsEnabled() {
		return ""
	}

	return `## Spamoor Transaction Spammer

Control and monitor Spamoor transaction spammers. Spamoor is a powerful Ethereum
transaction generator for testnets with a daemon mode and REST API.

` + "```python" + `
from ethpandaops import spamoor

# List all spammer instances
instances = spamoor.list_instances("http://localhost:8080")

# Get details of a specific spammer
instance = spamoor.get_instance("http://localhost:8080", 1)
print(f"Spammer status: {instance['status']}")

# Start/stop spammers
spamoor.start_instance("http://localhost:8080", 1)
spamoor.stop_instance("http://localhost:8080", 1)

# Get wallet information
wallets = spamoor.get_wallets("http://localhost:8080")

# Generate deep links
link = spamoor.link_instance("http://localhost:8080", 1)
print(f"View in Spamoor: {link}")
` + "```" + `
`
}

// SetLogger sets the logger for the plugin.
func (p *Plugin) SetLogger(log logrus.FieldLogger) {
	p.log = log.WithField("plugin", "spamoor")
}

// RegisterResources is a no-op since Spamoor uses direct API calls.
func (p *Plugin) RegisterResources(_ logrus.FieldLogger, _ plugin.ResourceRegistry) error {
	return nil
}

func (p *Plugin) Start(_ context.Context) error { return nil }

func (p *Plugin) Stop(_ context.Context) error { return nil }
