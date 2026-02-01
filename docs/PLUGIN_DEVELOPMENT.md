# Plugin Development Guide

This guide walks you through creating a new datasource plugin for the ethpandaops MCP server. Plugins extend the server's capabilities by adding support for new data sources that can be queried from Python sandboxes.

## Table of Contents

- [Overview](#overview)
- [Plugin Architecture](#plugin-architecture)
- [Step-by-Step Guide](#step-by-step-guide)
- [Plugin Interface](#plugin-interface)
- [Configuration](#configuration)
- [Python Module](#python-module)
- [Proxy Handler](#proxy-handler)
- [Testing Your Plugin](#testing-your-plugin)
- [Best Practices](#best-practices)
- [Example: Creating a Simple Plugin](#example-creating-a-simple-plugin)

## Overview

A plugin in ethpandaops/mcp is a self-contained package that:

- Defines its own configuration schema
- Provides query examples for semantic search
- Exposes a Python module for sandbox execution
- Registers MCP resources (optional)
- Integrates with the credential proxy for secure data access

## Plugin Architecture

```
plugins/{name}/
├── plugin.go        # Main plugin implementation
├── config.go        # Configuration struct and validation
├── examples.go      # Embeds examples.yaml
├── examples.yaml    # Query examples for semantic search
└── python/
    └── {name}.py    # Python module for sandbox

pkg/proxy/handlers/  # (if using proxy)
└── {name}.go        # Reverse proxy handler
```

## Step-by-Step Guide

### 1. Create Plugin Directory Structure

```bash
mkdir -p plugins/{your-plugin-name}/python
```

### 2. Implement the Plugin Interface

Create `plugins/{name}/plugin.go`:

```go
package {name}

import (
    "context"
    "encoding/json"
    
    "github.com/sirupsen/logrus"
    "gopkg.in/yaml.v3"
    
    "github.com/ethpandaops/mcp/pkg/plugin"
    "github.com/ethpandaops/mcp/pkg/types"
)

// Compile-time interface check
var _ plugin.Plugin = (*Plugin)(nil)

type Plugin struct {
    cfg Config
    log logrus.FieldLogger
}

func New() *Plugin {
    return &Plugin{}
}

func (p *Plugin) Name() string { return "{name}" }

func (p *Plugin) Init(rawConfig []byte) error {
    return yaml.Unmarshal(rawConfig, &p.cfg)
}

func (p *Plugin) ApplyDefaults() {
    // Set default values
    if p.cfg.Timeout == 0 {
        p.cfg.Timeout = 30
    }
}

func (p *Plugin) Validate() error {
    // Validate configuration
    if len(p.cfg.Instances) == 0 {
        return plugin.ErrNoValidConfig
    }
    return nil
}

// SandboxEnv returns credential-free environment variables
func (p *Plugin) SandboxEnv() (map[string]string, error) {
    type instanceInfo struct {
        Name string `json:"name"`
        URL  string `json:"url"`
    }
    
    infos := make([]instanceInfo, 0, len(p.cfg.Instances))
    for _, inst := range p.cfg.Instances {
        infos = append(infos, instanceInfo{
            Name: inst.Name,
            URL:  inst.URL,
        })
    }
    
    infosJSON, err := json.Marshal(infos)
    if err != nil {
        return nil, err
    }
    
    return map[string]string{
        "ETHPANDAOPS_{NAME}_DATASOURCES": string(infosJSON),
    }, nil
}

func (p *Plugin) DatasourceInfo() []types.DatasourceInfo {
    infos := make([]types.DatasourceInfo, 0, len(p.cfg.Instances))
    for _, inst := range p.cfg.Instances {
        infos = append(infos, types.DatasourceInfo{
            Type: "{name}",
            Name: inst.Name,
            Metadata: map[string]string{
                "url": inst.URL,
            },
        })
    }
    return infos
}

func (p *Plugin) Examples() map[string]types.ExampleCategory {
    return queryExamples
}

func (p *Plugin) PythonAPIDocs() map[string]types.ModuleDoc {
    return map[string]types.ModuleDoc{
        "{name}": {
            Description: "Query {Name} for data",
            Functions: map[string]types.FunctionDoc{
                "query": {
                    Signature:   "{name}.query(instance: str, query: str)",
                    Description: "Execute a query",
                    Parameters: map[string]string{
                        "instance": "Instance name",
                        "query":    "Query string",
                    },
                },
            },
        },
    }
}

func (p *Plugin) GettingStartedSnippet() string {
    return "## {Name}\n\nUse `{name}.query()` to fetch data."
}

func (p *Plugin) RegisterResources(log logrus.FieldLogger, reg plugin.ResourceRegistry) error {
    p.log = log.WithField("plugin", "{name}")
    // Register custom resources if needed
    return nil
}

func (p *Plugin) Start(ctx context.Context) error {
    // Async initialization (optional)
    return nil
}

func (p *Plugin) Stop(ctx context.Context) error {
    // Cleanup (optional)
    return nil
}
```

### 3. Define Configuration

Create `plugins/{name}/config.go`:

```go
package {name}

// Config holds the plugin configuration
type Config struct {
    Instances []InstanceConfig `yaml:"instances"`
    Timeout   int              `yaml:"timeout"`
}

// InstanceConfig represents a single instance
type InstanceConfig struct {
    Name string `yaml:"name"`
    URL  string `yaml:"url"`
}
```

### 4. Add Query Examples

Create `plugins/{name}/examples.go`:

```go
package {name}

import "github.com/ethpandaops/mcp/pkg/types"

//go:embed examples.yaml
var examplesYAML []byte

var queryExamples map[string]types.ExampleCategory

func init() {
    queryExamples = types.LoadExamples(examplesYAML)
}
```

Create `plugins/{name}/examples.yaml`:

```yaml
basic_queries:
  name: Basic Queries
  description: Simple query examples
  examples:
    - name: List all instances
      description: Get a list of available instances
      code: |
        import {name}
        ds = {name}.list_datasources()
        print(ds)
      tags: ["list", "discovery"]
```

### 5. Create Python Module

Create `plugins/{name}/python/{name}.py`:

```python
"""{Name} datasource module for ethpandaops MCP.

This module provides access to {Name} data from sandboxed Python code.
"""
import json
import os
from typing import Any

# Load datasource info from environment (no credentials!)
_DATASOURCES_JSON = os.environ.get("ETHPANDAOPS_{NAME}_DATASOURCES", "[]")
_DATASOURCES = json.loads(_DATASOURCES_JSON)

# Get proxy configuration from environment
_PROXY_URL = os.environ.get("ETHPANDAOPS_PROXY_URL", "")
_PROXY_TOKEN = os.environ.get("ETHPANDAOPS_PROXY_TOKEN", "")


def list_datasources() -> list[dict]:
    """List available {Name} datasources.
    
    Returns:
        List of datasource info dicts with 'name' and 'url' keys.
    """
    return _DATASOURCES


def query(instance: str, query: str) -> Any:
    """Execute a query against a {Name} instance.
    
    Args:
        instance: The instance name to query
        query: The query string
        
    Returns:
        Query results
    """
    import requests
    
    # Find the instance URL
    inst = next((d for d in _DATASOURCES if d["name"] == instance), None)
    if not inst:
        raise ValueError(f"Unknown instance: {instance}")
    
    # Call through the proxy
    url = f"{_PROXY_URL}/{name}/{instance}/query"
    headers = {"Authorization": f"Bearer {_PROXY_TOKEN}"}
    
    response = requests.post(url, json={"query": query}, headers=headers)
    response.raise_for_status()
    
    return response.json()
```

### 6. Register the Plugin

Edit `pkg/server/builder.go`:

```go
import (
    // ... existing imports ...
    yourplugin "github.com/ethpandaops/mcp/plugins/{name}"
)

func (b *Builder) buildPluginRegistry() (*plugin.Registry, error) {
    reg := plugin.NewRegistry(b.log)
    
    // ... existing plugins ...
    reg.Add(yourplugin.New())
    
    // ... rest of the function ...
}
```

### 7. Update Python Package

Edit `sandbox/ethpandaops/ethpandaops/__init__.py`:

```python
# Add lazy import for your module
def __getattr__(name):
    if name == "{name}":
        from . import {name} as mod
        return mod
    # ... existing imports ...
```

Copy your Python module:

```bash
cp plugins/{name}/python/{name}.py sandbox/ethpandaops/ethpandaops/
```

## Plugin Interface

The `plugin.Plugin` interface requires these methods:

| Method | Purpose |
|--------|---------|
| `Name()` | Returns the plugin identifier (lowercase) |
| `Init(rawConfig)` | Parses YAML config into your struct |
| `ApplyDefaults()` | Sets default configuration values |
| `Validate()` | Validates the configuration |
| `SandboxEnv()` | Returns env vars for sandbox (no credentials!) |
| `DatasourceInfo()` | Returns metadata for datasources:// resource |
| `Examples()` | Returns query examples for semantic search |
| `PythonAPIDocs()` | Returns Python API documentation |
| `GettingStartedSnippet()` | Returns Markdown for getting-started guide |
| `RegisterResources()` | Registers custom MCP resources |
| `Start()` | Async initialization (e.g., schema discovery) |
| `Stop()` | Cleanup on shutdown |

### Optional Interfaces

Your plugin can optionally implement:

- **`plugin.ProxyAware`** - Receive the proxy client for schema discovery
- **`plugin.CartographoorAware`** - Receive the cartographoor client for network discovery
- **`plugin.DefaultEnabled`** - Enable the plugin without explicit config

## Configuration

### Configuration Schema

Plugins define their own configuration schema in `config.go`. The schema is placed under the `plugins:` key in the main config:

```yaml
plugins:
  {name}:
    instances:
      - name: prod
        url: https://api.example.com
    timeout: 30
```

### Environment Variables

Use `${VAR_NAME}` syntax for environment variable substitution:

```yaml
plugins:
  {name}:
    instances:
      - name: prod
        url: "${API_URL}"
```

## Python Module

### Key Principles

1. **No credentials in the sandbox** - Python modules read only metadata from environment variables
2. **Connect via proxy** - All data access flows through the credential proxy
3. **Use standard libraries** - Prefer `requests`, `pandas`, `numpy`

### Environment Variables Available

| Variable | Description |
|----------|-------------|
| `ETHPANDAOPS_{NAME}_DATASOURCES` | JSON array of datasource metadata |
| `ETHPANDAOPS_PROXY_URL` | URL of the credential proxy |
| `ETHPANDAOPS_PROXY_TOKEN` | JWT token for proxy authentication |
| `ETHPANDAOPS_STORAGE_ENDPOINT` | S3-compatible storage endpoint |
| `ETHPANDAOPS_STORAGE_BUCKET` | S3 bucket for outputs |

## Proxy Handler

If your plugin needs to proxy requests through the credential proxy, create a handler:

Create `pkg/proxy/handlers/{name}.go`:

```go
package handlers

import (
    "net/http"
    "net/http/httputil"
    "net/url"
)

// {Name}Handler creates a reverse proxy for {Name}
func {Name}Handler(targetURL string) http.Handler {
    target, _ := url.Parse(targetURL)
    proxy := httputil.NewSingleHostReverseProxy(target)
    
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        proxy.ServeHTTP(w, r)
    })
}
```

Register in `pkg/proxy/proxy.go`:

```go
func (p *Proxy) Start() error {
    // ... existing handlers ...
    
    http.Handle("/{name}/", handlers.{Name}Handler(p.opts.{Name}URL))
    
    // ... rest of function ...
}
```

## Testing Your Plugin

### Unit Tests

Create `plugins/{name}/plugin_test.go`:

```go
package {name}

import (
    "testing"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestPlugin_Init(t *testing.T) {
    p := New()
    
    config := `
instances:
  - name: test
    url: https://test.example.com
`
    err := p.Init([]byte(config))
    require.NoError(t, err)
    
    assert.Len(t, p.cfg.Instances, 1)
    assert.Equal(t, "test", p.cfg.Instances[0].Name)
}

func TestPlugin_Validate(t *testing.T) {
    p := New()
    
    // Empty config should return ErrNoValidConfig
    err := p.Validate()
    assert.Equal(t, plugin.ErrNoValidConfig, err)
}
```

### Integration Tests

Run the full test suite:

```bash
make test
```

Test sandbox execution:

```bash
make docker-sandbox
make test-sandbox
```

## Best Practices

### Security

- **Never pass credentials to the sandbox** - Always use the proxy
- **Validate all inputs** - Check instance names against configured datasources
- **Use timeouts** - Set reasonable query timeouts

### Configuration

- **Provide sensible defaults** - Use `ApplyDefaults()` for optional values
- **Return `ErrNoValidConfig`** - When no valid instances are configured
- **Support env var substitution** - For URLs, credentials (in proxy config)

### Documentation

- **Write clear examples** - Each example should be runnable
- **Document the Python API** - Use `PythonAPIDocs()` for function signatures
- **Add getting started content** - Help users understand your plugin

### Code Organization

- **Keep plugin self-contained** - All plugin code in `plugins/{name}/`
- **Copy existing patterns** - Use `prometheus/` for simple plugins, `clickhouse/` for complex ones
- **Use embed for examples** - Keep examples in YAML, embed with `//go:embed`

## Example: Creating a Simple Plugin

Let's create a plugin for a hypothetical metrics API called "MetricsDB":

### 1. Create files

```bash
mkdir -p plugins/metricsdb/python
touch plugins/metricsdb/{plugin,config,examples}.go
touch plugins/metricsdb/examples.yaml
touch plugins/metricsdb/python/metricsdb.py
```

### 2. Implement (following the templates above)

See the existing plugins (`prometheus/`, `loki/`) for complete working examples.

### 3. Register and test

```bash
# Add to builder.go
# Update __init__.py
# Copy Python module

# Build and test
make build
make lint
make test
```

## Need Help?

- Check existing plugins: `plugins/clickhouse/`, `plugins/prometheus/`, `plugins/loki/`
- Review the plugin interface: `pkg/plugin/plugin.go`
- Open an issue on GitHub for questions
