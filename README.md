# xatu-mcp

An MCP (Model Context Protocol) server for Ethereum network analytics via [Xatu](https://github.com/ethpandaops/xatu) data.

The overall idea behind this server is to empower agents with an MCP code execution environment (as opposed to just providing a query interface). This allows for improve token usage, and while also enabling more complex tasks to be performed.

Read more here: https://www.anthropic.com/engineering/code-execution-with-mcp

## Overview

xatu-mcp enables AI assistants to analyze Ethereum blockchain data by providing:

- **Sandboxed Python execution** with pre-installed data analysis libraries
- **ClickHouse access** to raw and aggregated blockchain data across multiple networks
- **Schema introspection** for understanding table structures
- **Query examples** to guide data exploration
- **File storage** for charts and outputs via S3-compatible storage

## Features

### Tools

| Tool | Description |
|------|-------------|
| `execute_python` | Execute Python code in an isolated sandbox with the `xatu` library |
| `search_examples` | Search for query examples and patterns |

### Resources

| Resource | Description |
|----------|-------------|
| `api://xatu` | Xatu library API documentation |
| `networks://active` | Compact list of active Ethereum networks and devnet groups |
| `networks://all` | All Ethereum networks including inactive ones |
| `networks://{name}` | Details for a specific network (e.g., `networks://mainnet`) |
| `networks://{group}` | All networks in a devnet group (e.g., `networks://fusaka`) |
| `examples://queries` | Common query patterns |
| `datasources://list` | List all Grafana datasources |
| `datasources://clickhouse` | List ClickHouse datasources |
| `datasources://prometheus` | List Prometheus datasources |
| `datasources://loki` | List Loki datasources |
| `clickhouse://tables` | List all ClickHouse tables (if schema discovery enabled) |
| `clickhouse://tables/{table}` | Detailed schema for a specific table |

### Available ClickHouse Clusters

| Cluster | Description | Networks |
|---------|-------------|----------|
| `xatu` | Production raw data | mainnet, sepolia, holesky, hoodi |
| `xatu-experimental` | Devnet data | pectra-devnet-6, etc. |
| `xatu-cbt` | Aggregated/CBT tables | mainnet, sepolia, holesky |

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/ethpandaops/xatu-mcp.git
cd xatu-mcp

# Build
make build

# Or with Go directly
go build -o xatu-mcp ./cmd/xatu-mcp
```

### Docker

```bash
# Build Docker image
make docker

# Or directly
docker build -t xatu-mcp:latest .
```

## Configuration

Copy the example configuration and customize it:

```bash
cp config.example.yaml config.yaml
```

### Environment Variables

The configuration supports environment variable substitution using `${VAR_NAME}` syntax:

```yaml
grafana:
  url: "${GRAFANA_URL}"
  service_token: "${GRAFANA_SERVICE_TOKEN}"
```

Required environment variables depend on your configuration:
- `GRAFANA_URL`, `GRAFANA_SERVICE_TOKEN` - Grafana connection (required)
- `S3_ENDPOINT`, `S3_ACCESS_KEY`, `S3_SECRET_KEY` - Storage credentials
- `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET` - OAuth (if auth enabled)

## Usage

### Start the Server

```bash
# Stdio transport (for local MCP clients)
xatu-mcp serve

# SSE transport (for web-based clients)
xatu-mcp serve --transport sse --port 8080

# Streamable HTTP transport
xatu-mcp serve --transport streamable-http --port 8080
```

### Docker Compose

For a complete local development environment:

```bash
docker-compose up -d
```

This starts:
- MCP server on port 8080
- MinIO for S3-compatible storage (ports 9000/9001)

### Claude Desktop Integration

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "xatu": {
      "command": "/path/to/xatu-mcp",
      "args": ["serve"],
      "env": {
        "CONFIG_PATH": "/path/to/config.yaml",
        "GRAFANA_URL": "https://grafana.example.com",
        "GRAFANA_SERVICE_TOKEN": "your-service-token"
      }
    }
  }
}
```

## Sandbox Environment

Python code executes in an isolated Docker container with:

### Pre-installed Libraries

- **Data Analysis**: pandas, numpy, polars
- **Visualization**: matplotlib, seaborn, plotly
- **ClickHouse**: clickhouse-connect
- **HTTP Client**: httpx
- **S3 Client**: boto3

### The `xatu` Library

```python
from xatu import clickhouse, prometheus, loki, storage

# List available ClickHouse datasources
datasources = clickhouse.list_datasources()
print(datasources)  # [{'uid': 'abc123', 'name': 'xatu-clickhouse', ...}]

# Query ClickHouse using datasource UID
# Use datasources://clickhouse resource to find available UIDs
df = clickhouse.query("your-datasource-uid", """
    SELECT
        slot,
        block_root,
        proposer_index
    FROM beacon_api_eth_v1_events_block
    WHERE meta_network_name = 'mainnet'
      AND slot_start_date_time >= now() - INTERVAL 1 HOUR
    LIMIT 10
""")

# Query Prometheus metrics (uses datasource UID)
result = prometheus.query("your-prometheus-uid", "up")

# Query Loki logs (uses datasource UID)
logs = loki.query("your-loki-uid", '{job="beacon"}')

# Save and upload a chart
import matplotlib.pyplot as plt
plt.figure(figsize=(10, 6))
plt.plot(df['slot'], df['proposer_index'])
plt.savefig('/workspace/chart.png')

# Upload to get a public URL
url = storage.upload('/workspace/chart.png')
print(f"Chart: {url}")
```

### Output Files

Files written to `/workspace/` persist within a session and can be uploaded to S3 using `storage.upload()` for public access.

## Authentication

When deploying as a public service, enable GitHub OAuth authentication:

```yaml
auth:
  enabled: true
  github:
    client_id: "${GITHUB_CLIENT_ID}"
    client_secret: "${GITHUB_CLIENT_SECRET}"
  allowed_orgs:
    - "ethpandaops"
```

## Observability

### Metrics

Prometheus metrics are exposed on port 9090 (configurable):

- `xatu_mcp_tool_calls_total` - Total tool calls by tool name and status
- `xatu_mcp_tool_call_duration_seconds` - Tool call duration histogram
- `xatu_mcp_sandbox_executions_total` - Sandbox executions by backend and status
- `xatu_mcp_sandbox_duration_seconds` - Sandbox execution duration histogram
- `xatu_mcp_clickhouse_queries_total` - ClickHouse queries by cluster and status
- `xatu_mcp_active_connections` - Current active MCP connections

### Health Endpoints

- `GET /health` - Health check
- `GET /ready` - Readiness check
- `GET /metrics` - Prometheus metrics

## Development

```bash
# Build
make build

# Run tests
make test

# Run linters
make lint

# Format code
make fmt

# Build Docker image
make docker

# Run with SSE transport
make run-sse
```

### Building the Sandbox Image

```bash
make docker-sandbox

# Or directly
docker build -t xatu-mcp-sandbox:latest ./sandbox
```

## Architecture

```
┌─────────────────┐     ┌──────────────────┐
│   MCP Client    │────▶│   xatu-mcp       │
│  (Claude, etc)  │     │    Server        │
└─────────────────┘     └────────┬─────────┘
                                 │
        ┌────────────────────────┼────────────────────────┐
        ▼                        ▼                        ▼
┌───────────────┐      ┌─────────────────┐      ┌─────────────────┐
│   Sandbox     │      │   ClickHouse    │      │   S3 Storage    │
│   (Docker/    │      │   Clusters      │      │   (MinIO/R2)    │
│    gVisor)    │      │                 │      │                 │
└───────────────┘      └─────────────────┘      └─────────────────┘
```

## License

MIT License - see [LICENSE](LICENSE) for details.
