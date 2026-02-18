---
name: query
description: Query Ethereum network data via ethpandaops MCP server. Use when analyzing blockchain data, block timing, attestations, validator performance, network health, or infrastructure metrics. Provides access to ClickHouse (blockchain data), Prometheus (metrics), Loki (logs), and Dora (explorer APIs).
argument-hint: <query or question>
user-invocable: false
allowed-tools: Bash(ethpandaops-mcp:*), mcp__ethpandaops-mcp__execute_python, mcp__ethpandaops-mcp__search_examples, mcp__ethpandaops-mcp__search_runbooks, mcp__ethpandaops-mcp__manage_session
---

# ethpandaops MCP — Ethereum Data Analysis

Query Ethereum network data through the ethpandaops platform. Execute Python code in sandboxed containers with access to ClickHouse blockchain data, Prometheus metrics, Loki logs, and Dora explorer APIs.

## Detect Your Mode

Check which tools are available to you:

- **MCP mode:** You have `execute_python`, `search_examples`, `search_runbooks`, and `manage_session` MCP tools, plus MCP resources like `datasources://list`. Use MCP tool calls directly.
- **CLI mode:** You do NOT have MCP tools. Use `ethpandaops-mcp` CLI commands via Bash instead.

> If unsure, try listing your available tools. If you see `execute_python` in the list, use MCP mode. Otherwise, use CLI mode.

## Workflow

1. **Discover** — Find available datasources and table schemas
2. **Find patterns** — Search for query examples and runbooks
3. **Execute** — Run Python code in sandboxed containers
4. **Visualize** — Upload charts/outputs for shareable URLs

---

## Discovering Data Sources

<details>
<summary><strong>MCP mode</strong></summary>

Read MCP resources directly:
- `datasources://list` — All datasources (ClickHouse, Prometheus, Loki)
- `datasources://clickhouse` — ClickHouse clusters only
- `datasources://prometheus` — Prometheus instances only
- `datasources://loki` — Loki instances only
- `clickhouse://tables` — List all ClickHouse tables
- `clickhouse://tables/{table_name}` — Specific table schema
- `ethpandaops://getting-started` — Read this first for cluster rules and workflow guidance

</details>

<details>
<summary><strong>CLI mode</strong></summary>

```bash
# List all datasources (ClickHouse, Prometheus, Loki)
ethpandaops-mcp datasources

# Filter by type
ethpandaops-mcp datasources --type clickhouse

# List ClickHouse tables
ethpandaops-mcp tables

# Show specific table schema
ethpandaops-mcp tables fct_block_canonical
```

</details>

## Finding Query Patterns

<details>
<summary><strong>MCP mode</strong></summary>

Use MCP tools:
- `search_examples(query="block arrival time")` — Find SQL/PromQL/LogQL query snippets
- `search_examples(query="attestation participation", category="attestations")` — Filter by category
- `search_runbooks(query="network not finalizing")` — Multi-step investigation procedures
- `search_runbooks(query="slow queries", tag="performance")` — Filter by tag

</details>

<details>
<summary><strong>CLI mode</strong></summary>

```bash
# Search for example queries
ethpandaops-mcp search examples --query "block arrival time"
ethpandaops-mcp search examples --query "attestation participation" --category attestations

# Search for investigation runbooks
ethpandaops-mcp search runbooks --query "network not finalizing"
ethpandaops-mcp search runbooks --query "slow queries" --tag performance
```

</details>

## Executing Python Code

<details>
<summary><strong>MCP mode</strong></summary>

Use `execute_python` tool:
- `execute_python(code="...", session_id="abc123", timeout=60)`
- **Always** pass `session_id` from previous responses to reuse sessions
- Code runs in a sandboxed container with the `ethpandaops` library pre-installed

</details>

<details>
<summary><strong>CLI mode</strong></summary>

```bash
# Inline code (simple one-liners)
ethpandaops-mcp run --code 'from ethpandaops import clickhouse; print(clickhouse.list_datasources())'

# From file (preferred for multi-line queries)
ethpandaops-mcp run --file /tmp/query.py

# With session reuse (faster startup, persistent /workspace)
ethpandaops-mcp run --file /tmp/query.py --session-id abc123

# With custom timeout
ethpandaops-mcp run --file /tmp/query.py --timeout 120

# JSON output for structured parsing
ethpandaops-mcp run --file /tmp/query.py --json
```

**Important:** Write Python code to a file first, then run with `--file`. This avoids shell escaping issues with complex queries.

</details>

## Session Management

<details>
<summary><strong>MCP mode</strong></summary>

Use `manage_session` tool:
- `manage_session(operation="list")` — View active sessions
- `manage_session(operation="create")` — Create a new session
- `manage_session(operation="destroy", session_id="...")` — Free a session

</details>

<details>
<summary><strong>CLI mode</strong></summary>

```bash
ethpandaops-mcp session list        # View active sessions
ethpandaops-mcp session create      # Pre-create a session
ethpandaops-mcp session destroy ID  # Free a session
```

</details>

---

## The ethpandaops Python Library

Everything below applies to **both modes** — the Python code you write is identical regardless of how you invoke it.

### ClickHouse — Blockchain Data

```python
from ethpandaops import clickhouse

# List available clusters
clusters = clickhouse.list_datasources()
# Returns: [{"name": "xatu", "database": "default"}, {"name": "xatu-cbt", ...}]

# Query data (returns pandas DataFrame)
df = clickhouse.query("xatu-cbt", """
    SELECT
        slot,
        avg(seen_slot_start_diff) as avg_arrival_ms
    FROM mainnet.fct_block_first_seen_by_node
    WHERE slot_start_date_time >= now() - INTERVAL 1 HOUR
    GROUP BY slot
    ORDER BY slot DESC
""")

# Parameterized queries
df = clickhouse.query("xatu", "SELECT * FROM blocks WHERE slot > {slot}", {"slot": 1000})
```

**Cluster selection:**
- `xatu-cbt` — Pre-aggregated tables (faster, use for metrics)
- `xatu` — Raw event data (use for detailed analysis)

**Required filters:**
- ALWAYS filter on partition key: `slot_start_date_time >= now() - INTERVAL X HOUR`
- Filter by network: `meta_network_name = 'mainnet'` or use schema like `mainnet.table_name`

### Prometheus — Infrastructure Metrics

```python
from ethpandaops import prometheus

# List instances
instances = prometheus.list_datasources()

# Instant query
result = prometheus.query("ethpandaops", "up")

# Range query
result = prometheus.query_range(
    "ethpandaops",
    "rate(http_requests_total[5m])",
    start="now-1h",
    end="now",
    step="1m"
)
```

**Time formats:** RFC3339 or relative (`now`, `now-1h`, `now-30m`)

### Loki — Log Data

```python
from ethpandaops import loki

# List instances
instances = loki.list_datasources()

# Query logs
logs = loki.query(
    "ethpandaops",
    '{app="beacon-node"} |= "error"',
    start="now-1h",
    limit=100
)
```

### Dora — Beacon Chain Explorer

```python
from ethpandaops import dora

# Get network health
overview = dora.get_network_overview("mainnet")
print(f"Current epoch: {overview['current_epoch']}")
print(f"Active validators: {overview['active_validator_count']}")

# Check finality
epochs_behind = overview['current_epoch'] - overview.get('finalized_epoch', 0)
if epochs_behind > 2:
    print(f"Warning: {epochs_behind} epochs behind finality")

# Generate explorer links
link = dora.link_validator("mainnet", "12345")
link = dora.link_slot("mainnet", "9000000")
link = dora.link_epoch("mainnet", 280000)
```

### Storage — Upload Outputs

```python
from ethpandaops import storage

# Save visualization
import matplotlib.pyplot as plt
plt.savefig("/workspace/chart.png")

# Upload for public URL
url = storage.upload("/workspace/chart.png")
print(f"Chart URL: {url}")

# List uploaded files
files = storage.list_files()
```

## Session Behavior

**Critical:** Each execution runs in a **fresh Python process**. Variables do NOT persist between calls.

- **Files persist:** Save to `/workspace/` to share data between calls
- **Reuse sessions:** Pass `session_id` from previous output for faster startup and workspace persistence

### Multi-Step Analysis Pattern

```python
# Step 1: Query and save
from ethpandaops import clickhouse
df = clickhouse.query("xatu-cbt", "SELECT ...")
df.to_parquet("/workspace/data.parquet")
```

```python
# Step 2: Load and visualize (reuse session)
import pandas as pd
import matplotlib.pyplot as plt
from ethpandaops import storage

df = pd.read_parquet("/workspace/data.parquet")
plt.figure(figsize=(12, 6))
plt.plot(df["slot"], df["value"])
plt.savefig("/workspace/chart.png")
url = storage.upload("/workspace/chart.png")
print(f"Chart: {url}")
```

## Error Handling

ClickHouse errors include actionable suggestions:
- Missing date filter -> "Add `slot_start_date_time >= now() - INTERVAL X HOUR`"
- Wrong cluster -> "Use xatu-cbt for aggregated metrics"
- Query timeout -> Break into smaller time windows

Default execution timeout is 60s, max 600s. For large analyses:
- Search examples before writing complex queries from scratch
- Break work into smaller time windows
- Save intermediate results to `/workspace/`

## Rules

- Always filter ClickHouse queries on partition keys (`slot_start_date_time`)
- Use `xatu-cbt` for pre-aggregated metrics, `xatu` for raw event data
- Search examples before writing complex queries from scratch
- Upload visualizations with `storage.upload()` for shareable URLs
- NEVER just copy/paste/recite base64 of images. You MUST save the image to the workspace and upload it to give it back to the user.
