# ethpandaops-mcp

An MCP server that provides AI assistants with Ethereum network analytics capabilities via [Xatu](https://github.com/ethpandaops/xatu) data.

Agents execute Python code in sandboxed containers with direct access to ClickHouse blockchain data, Prometheus metrics, Loki logs, and S3-compatible storage for outputs.

Read more: https://www.anthropic.com/engineering/code-execution-with-mcp

## Two Ways to Use

There are two ways to connect your AI assistant to ethpandaops data. Both provide the same capabilities — choose whichever fits your setup.

### Option A: MCP Server (recommended for most users)

Connect your AI assistant to a running MCP server instance. The server exposes tools and resources via the MCP protocol.

**Claude Code:**

```bash
# Add the MCP server
npx add-mcp ethpandaops-mcp -- http --url http://localhost:2480/mcp

# Install skills for query knowledge
npx skills add ethpandaops/mcp
```

Or manually add to `~/.claude.json` under `mcpServers`:

```json
{
  "ethpandaops-mcp": {
    "type": "http",
    "url": "http://localhost:2480/mcp"
  }
}
```

**Claude Desktop** — add to `~/Library/Application Support/Claude/claude_desktop_config.json`:

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

**Running the server:**

```bash
# Configure
cp config.example.yaml config.yaml
# Edit config.yaml with your datasource credentials

# Run (builds sandbox image, starts MinIO + proxy + MCP server)
docker-compose up -d
```

The server runs on port 2480 (streamable-http transport, configurable via `MCP_SERVER_PORT`).

### Option B: CLI Binary (for local-agent mode)

Use the `ethpandaops-mcp` binary directly from the command line. Your coding agent (e.g. Claude Code) invokes CLI commands via Bash — no MCP server process needed. The binary connects to a production credential proxy for datasource access.

```bash
# Authenticate
ethpandaops-mcp login

# Install skills so your agent knows the CLI commands
npx skills add ethpandaops/mcp

# Your agent can now run queries via Bash:
ethpandaops-mcp run --code 'from ethpandaops import clickhouse; print(clickhouse.list_datasources())'
ethpandaops-mcp run --file /tmp/query.py --session-id abc123
ethpandaops-mcp search examples --query "block timing"
ethpandaops-mcp datasources
ethpandaops-mcp tables fct_block_canonical
```

**Requirements:** Docker must be running locally (for the sandbox containers). The binary itself is a single static binary.

**CLI commands:**

| Command | Description |
|---------|-------------|
| `login` / `logout` | Authenticate with the credential proxy |
| `run --code '...'` / `run --file path.py` | Execute Python in a sandbox |
| `search examples --query "..."` | Search query examples |
| `search runbooks --query "..."` | Search investigation runbooks |
| `datasources` | List available ClickHouse/Prometheus/Loki datasources |
| `tables [name]` | List or inspect ClickHouse table schemas |
| `session list` / `create` / `destroy` | Manage persistent sandbox sessions |
| `serve` | Start the full MCP server (Option A) |

## Deployment Modes

See `docs/deployments.md` for dev, local-agent, and remote-agent deployment modes.

## Tools & Resources (MCP Server)

| Tool | Description |
|------|-------------|
| `execute_python` | Execute Python in a sandbox with the `ethpandaops` library |
| `search_examples` | Search for query examples and patterns |
| `search_runbooks` | Search investigation runbooks |
| `manage_session` | List, create, and destroy sandbox sessions |

Resources are available for getting started (`mcp://getting-started`), datasource discovery (`datasources://`), network info (`networks://`), table schemas (`clickhouse://`), and Python API docs (`python://ethpandaops`).

## Development

```bash
make build           # Build binary (produces ./ethpandaops-mcp)
make test            # Run tests
make lint            # Run linters
make docker          # Build Docker image
make docker-sandbox  # Build sandbox image
```

## License

MIT
