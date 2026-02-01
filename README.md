# ethpandaops-mcp

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/go-1.24-blue.svg)](https://golang.org)
[![Docker](https://img.shields.io/badge/docker-supported-blue.svg)](https://docker.com)

An MCP (Model Context Protocol) server that provides AI assistants with Ethereum network analytics capabilities via [Xatu](https://github.com/ethpandaops/xatu) data.

Agents execute Python code in sandboxed containers with direct access to ClickHouse blockchain data, Prometheus metrics, Loki logs, and S3-compatible storage for outputs.

Read more about the architecture: https://www.anthropic.com/engineering/code-execution-with-mcp

## Table of Contents

- [Features](#features)
- [Quick Start](#quick-start)
- [Installation](#installation)
- [Configuration](#configuration)
- [Deployment Modes](#deployment-modes)
- [Usage](#usage)
  - [Claude Code](#claude-code)
  - [Claude Desktop](#claude-desktop)
- [Tools & Resources](#tools--resources)
- [Development](#development)
- [Architecture](#architecture)
- [Contributing](#contributing)
- [License](#license)

## Features

- **🔒 Secure Sandboxed Execution** - Python code runs in isolated Docker/gVisor containers
- **📊 Multiple Datasources** - Query ClickHouse, Prometheus, and Loki through a unified interface
- **🔐 Credential Isolation** - Credentials never reach the sandbox; all access flows through a secure proxy
- **🔍 Semantic Search** - GGUF-based embedding model for finding relevant query examples
- **📚 Extensible Plugin System** - Add new datasources by implementing the plugin interface
- **🌐 Multiple Transports** - Support for stdio, SSE, and HTTP transports
- **🔑 GitHub OAuth** - Secure authentication for HTTP transports
- **💾 Session Management** - Persistent sandbox sessions for maintaining state across calls

## Quick Start

### Using Docker Compose (Recommended)

```bash
# Clone the repository
git clone https://github.com/ethpandaops/mcp.git
cd mcp

# Configure
cp config.example.yaml config.yaml
# Edit config.yaml with your datasource credentials

# Run (builds sandbox image, starts MinIO + MCP server)
docker-compose up -d
```

The server runs on port 2480 (SSE transport) with MinIO on ports 2400/2401.

### Using Pre-built Binaries

```bash
# Download the latest release
curl -L -o mcp https://github.com/ethpandaops/mcp/releases/latest/download/mcp-$(uname -s)-$(uname -m)
chmod +x mcp

# Configure and run
cp config.example.yaml config.yaml
./mcp serve --transport sse --port 2480
```

### Building from Source

```bash
# Build the binary
make build

# Build the sandbox Docker image
make docker-sandbox

# Run
make run-sse
```

## Installation

### Requirements

- **Go 1.24+** (for building from source)
- **Docker** (for sandbox execution)
- **ClickHouse/Prometheus/Loki** (datasources to query)
- **S3-compatible storage** (for output files)

### Docker Images

| Image | Description |
|-------|-------------|
| `ethpandaops/mcp:latest` | MCP server |
| `ethpandaops/mcp-sandbox:latest` | Sandbox execution environment |

## Configuration

Create a `config.yaml` file based on `config.example.yaml`:

```yaml
server:
  host: "0.0.0.0"
  port: 2480

# Plugin configuration
plugins:
  clickhouse:
    schema_discovery:
      enabled: true
      refresh_interval: 15m

# Sandbox configuration
sandbox:
  backend: docker
  image: "ethpandaops-mcp-sandbox:latest"
  timeout: 60

# S3-compatible storage
storage:
  endpoint: "${S3_ENDPOINT}"
  access_key: "${S3_ACCESS_KEY}"
  secret_key: "${S3_SECRET_KEY}"
  bucket: "ethpandaops-mcp-outputs"

# Proxy configuration (for production deployments)
proxy:
  url: "http://localhost:18081"
```

See [config.example.yaml](config.example.yaml) for all available options.

## Deployment Modes

The MCP server supports three deployment modes:

| Mode | Use Case | Proxy | Sandbox |
|------|----------|-------|---------|
| **Development** | Local iteration | Local (no auth) | Docker |
| **Local Agent** | Prod datasources, local execution | Remote (JWT) | Docker |
| **Remote Agent** | Hosted deployment | Remote (JWT) | gVisor |

See [docs/deployments.md](docs/deployments.md) for detailed configuration.

## Usage

### Claude Code

Add to `~/.claude.json` under `mcpServers`:

```json
{
  "mcpServers": {
    "ethpandaops-mcp": {
      "type": "http",
      "url": "http://localhost:2480/mcp"
    }
  }
}
```

### Claude Desktop

Add to `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "ethpandaops-mcp": {
      "type": "http",
      "url": "http://localhost:2480/mcp"
    }
  }
}
```

### Skills

Install skills to give Claude knowledge about querying Ethereum data:

```bash
npx skills add ethpandaops/mcp
```

This installs the `query` skill which provides background knowledge for using the MCP tools effectively.

## Tools & Resources

### Tools

| Tool | Description |
|------|-------------|
| `execute_python` | Execute Python code in a sandbox with the `ethpandaops` library |
| `search_examples` | Search for query examples and patterns using semantic search |
| `search_runbooks` | Search diagnostic runbooks for common issues |
| `manage_session` | Manage persistent sandbox sessions |

### Resources

| Resource | Description |
|----------|-------------|
| `mcp://getting-started` | Getting started guide with workflow tips |
| `datasources://list` | All configured datasources |
| `networks://active` | Active Ethereum networks |
| `clickhouse://tables` | ClickHouse table schemas |
| `examples://queries` | Common query patterns |
| `python://ethpandaops` | Python library API documentation |

### Example Usage

```python
# Read the getting-started guide first
read_resource("mcp://getting-started")

# Execute Python to query ClickHouse
execute_python("""
import clickhouse
import pandas as pd

# Query the xatu cluster
df = clickhouse.query("xatu", """
    SELECT 
        slot_start_date_time,
        COUNT(*) as block_count
    FROM fct_block_canonical
    WHERE slot_start_date_time >= now() - INTERVAL 1 DAY
    GROUP BY slot_start_date_time
    ORDER BY slot_start_date_time
""")

print(df)
""")
```

## Development

### Build

```bash
make build           # Build binary
make build-linux     # Build for Linux (amd64)
```

### Test

```bash
make test            # Run tests with race detector
make test-coverage   # Run tests with coverage report
```

### Lint

```bash
make lint            # Run linters
make lint-fix        # Run linters and auto-fix
make fmt             # Format code
```

### Docker

```bash
make docker          # Build Docker image
make docker-sandbox  # Build sandbox image
```

### Run

```bash
make run             # Run with stdio transport
make run-sse         # Run with SSE transport on port 2480
make run-docker      # Run with docker-compose
```

### Evaluation Tests

The project includes an LLM evaluation harness in `tests/eval/`:

```bash
cd tests/eval
uv sync
uv run python -m scripts.run_eval  # Run all eval tests
uv run python -m scripts.repl      # Interactive REPL
```

## Architecture

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│   MCP Client    │────▶│  MCP Server      │────▶│  Credential     │
│  (Claude, etc.) │     │  (this project)  │     │  Proxy          │
└─────────────────┘     └──────────────────┘     └─────────────────┘
                               │                           │
                               ▼                           ▼
                        ┌──────────────────┐     ┌─────────────────┐
                        │  Sandbox         │     │  Datasources    │
                        │  (Docker/gVisor) │     │  (ClickHouse,   │
                        │                  │     │   Prometheus,   │
                        └──────────────────┘     │   Loki)         │
                                                 └─────────────────┘
```

For detailed architecture documentation, see [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

### Plugin System

The server uses a plugin architecture where each datasource is a self-contained plugin. To create a new plugin, see [docs/PLUGIN_DEVELOPMENT.md](docs/PLUGIN_DEVELOPMENT.md).

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

MIT License - see [LICENSE](LICENSE) for details.

---

Built with ❤️ by [ethpandaops](https://github.com/ethpandaops)
