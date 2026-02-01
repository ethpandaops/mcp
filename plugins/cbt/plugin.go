package cbt

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/ethpandaops/mcp/pkg/plugin"
	"github.com/ethpandaops/mcp/pkg/types"
)

// Compile-time interface check
var _ plugin.Plugin = (*Plugin)(nil)

type Plugin struct {
	cfg Config
}

func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string { return "cbt" }

func (p *Plugin) Init(rawConfig []byte) error {
	if err := yaml.Unmarshal(rawConfig, &p.cfg); err != nil {
		return err
	}

	// Filter out instances with empty required fields.
	validInstances := make([]InstanceConfig, 0, len(p.cfg.Instances))
	for _, inst := range p.cfg.Instances {
		if inst.Name != "" && inst.URL != "" {
			validInstances = append(validInstances, inst)
		}
	}
	p.cfg.Instances = validInstances

	// If no valid instances remain, signal that this plugin should be skipped.
	if len(p.cfg.Instances) == 0 {
		return plugin.ErrNoValidConfig
	}

	return nil
}

func (p *Plugin) ApplyDefaults() {
	for i := range p.cfg.Instances {
		if p.cfg.Instances[i].Timeout == 0 {
			p.cfg.Instances[i].Timeout = 60
		}
	}
}

func (p *Plugin) Validate() error {
	names := make(map[string]struct{}, len(p.cfg.Instances))
	for i, inst := range p.cfg.Instances {
		if inst.Name == "" {
			return fmt.Errorf("instances[%d].name is required", i)
		}
		if _, exists := names[inst.Name]; exists {
			return fmt.Errorf("instances[%d].name %q is duplicated", i, inst.Name)
		}
		names[inst.Name] = struct{}{}
		if inst.URL == "" {
			return fmt.Errorf("instances[%d].url is required", i)
		}
	}
	return nil
}

// SandboxEnv returns credential-free environment variables for the sandbox.
// Credentials are never passed to sandbox containers - they connect via
// the credential proxy instead.
func (p *Plugin) SandboxEnv() (map[string]string, error) {
	if len(p.cfg.Instances) == 0 {
		return nil, nil
	}

	// Return instance info without credentials.
	type instanceInfo struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		URL         string `json:"url"`
		UIURL       string `json:"ui_url"`
	}

	infos := make([]instanceInfo, 0, len(p.cfg.Instances))
	for _, inst := range p.cfg.Instances {
		infos = append(infos, instanceInfo{
			Name:        inst.Name,
			Description: inst.Description,
			URL:         inst.URL,
			UIURL:       inst.GetUIURL(),
		})
	}

	infosJSON, err := json.Marshal(infos)
	if err != nil {
		return nil, fmt.Errorf("marshaling CBT instance info: %w", err)
	}

	return map[string]string{
		"ETHPANDAOPS_CBT_INSTANCES": string(infosJSON),
	}, nil
}

func (p *Plugin) DatasourceInfo() []types.DatasourceInfo {
	infos := make([]types.DatasourceInfo, 0, len(p.cfg.Instances))
	for _, inst := range p.cfg.Instances {
		infos = append(infos, types.DatasourceInfo{
			Type:        "cbt",
			Name:        inst.Name,
			Description: inst.Description,
			Metadata: map[string]string{
				"url":    inst.URL,
				"ui_url": inst.GetUIURL(),
			},
		})
	}
	return infos
}

func (p *Plugin) Examples() map[string]types.ExampleCategory {
	result := make(map[string]types.ExampleCategory, len(queryExamples))
	for k, v := range queryExamples {
		result[k] = v
	}
	return result
}

// PythonAPIDocs returns the CBT module documentation.
func (p *Plugin) PythonAPIDocs() map[string]types.ModuleDoc {
	return map[string]types.ModuleDoc{
		"cbt": {
			Description: "Query CBT (ClickHouse Build Tool) for model metadata and transformation state.",
			Functions: map[string]types.FunctionDoc{
				"list_datasources": {
					Signature:   "cbt.list_datasources() -> list[dict]",
					Description: "List available CBT instances. Prefer datasources://cbt resource instead.",
					Returns:     "List of dicts with 'name', 'description', 'url', 'ui_url' keys",
				},
				"list_models": {
					Signature:   "cbt.list_models(instance: str = None, model_type: str = None, database: str = None) -> list[dict]",
					Description: "List all models from CBT. Optionally filter by type or database.",
					Parameters: map[string]string{
						"instance":    "CBT instance name (default: first available)",
						"model_type":  "Filter by type: 'external', 'transformation', or None for all",
						"database":    "Filter by database name",
					},
					Returns: "List of model dicts with 'database', 'table', 'type', 'dependencies', etc.",
				},
				"get_model": {
					Signature:   "cbt.get_model(database: str, table: str, instance: str = None) -> dict",
					Description: "Get detailed information about a specific model.",
					Parameters: map[string]string{
						"database": "Database name containing the model",
						"table":    "Table name of the model",
						"instance": "CBT instance name (default: first available)",
					},
					Returns: "Dict with model details including dependencies, interval config, schedules",
				},
				"get_transformation_status": {
					Signature:   "cbt.get_transformation_status(database: str, table: str, instance: str = None) -> dict",
					Description: "Query the processing status of a transformation model.",
					Parameters: map[string]string{
						"database": "Database name containing the model",
						"table":    "Table name of the model",
						"instance": "CBT instance name (default: first available)",
					},
					Returns: "Dict with 'total_intervals', 'min_position', 'max_position', 'total_gap_size'",
				},
				"get_model_ui_url": {
					Signature:   "cbt.get_model_ui_url(database: str, table: str, instance: str = None) -> str",
					Description: "Get deep link to CBT UI for a specific model.",
					Parameters: map[string]string{
						"database": "Database name containing the model",
						"table":    "Table name of the model",
						"instance": "CBT instance name (default: first available)",
					},
					Returns: "URL string to open the model in CBT UI",
				},
			},
		},
	}
}

// GettingStartedSnippet returns CBT-specific getting-started content.
func (p *Plugin) GettingStartedSnippet() string {
	return "## CBT (ClickHouse Build Tool)\n\n" +
		"CBT manages data transformation pipelines for ClickHouse.\n\n" +
		"### Model Types\n\n" +
		"- **External models**: Source data tables (e.g., beacon_blocks)\n" +
		"- **Transformation models**: Derived tables with SQL transformations\n\n" +
		"### Model Structure\n\n" +
		"Models are identified by database.table format:\n" +
		"- ethereum.beacon_blocks - External source\n" +
		"- analytics.hourly_stats - Transformation output\n\n" +
		"### Transformation Status\n\n" +
		"For incremental transformations, CBT tracks:\n" +
		"- **Position**: The ordering column (e.g., slot, timestamp)\n" +
		"- **Intervals**: Processed ranges [position, position + interval)\n" +
		"- **Gaps**: Missing data ranges needing backfill\n\n" +
		"Use cbt.get_transformation_status() to check processing progress."
}

func (p *Plugin) RegisterResources(_ logrus.FieldLogger, _ plugin.ResourceRegistry) error {
	// CBT doesn't register additional resources currently.
	return nil
}

func (p *Plugin) Start(_ context.Context) error { return nil }
func (p *Plugin) Stop(_ context.Context) error  { return nil }
