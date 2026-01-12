# xatu-mcp

An MCP server that provides AI assistants with Ethereum network analytics capabilities via [Xatu](https://github.com/ethpandaops/xatu) data.

Agents execute Python code in sandboxed containers with access to ClickHouse blockchain data, Prometheus metrics, Loki logs, and S3-compatible storage for outputs. All data queries are proxied through Grafana using datasource UIDs.

Read more: https://www.anthropic.com/engineering/code-execution-with-mcp

## Quick Start

```bash
# Build
make build
make docker-sandbox  # Required for Python execution

# Configure
cp config.example.yaml config.yaml
# Edit config.yaml with your Grafana URL and service token

# Run
./xatu-mcp serve                    # stdio transport (local)
./xatu-mcp serve -t sse -p 8080     # SSE transport (web)
```

## Claude Desktop

Add to `~/Library/Application Support/Claude/claude_desktop_config.json`:

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

## Tools & Resources

| Tool | Description |
|------|-------------|
| `execute_python` | Execute Python in a sandbox with the `xatu` library |
| `search_examples` | Search for query examples and patterns |

Resources are available for datasource discovery (`datasources://`), network info (`networks://`), table schemas (`clickhouse://`), and API docs (`api://xatu`).

## Development

```bash
make build           # Build binary
make test            # Run tests
make lint            # Run linters
make docker          # Build Docker image
make docker-sandbox  # Build sandbox image

# Local stack with MinIO
docker-compose up -d
```

## License

MIT
