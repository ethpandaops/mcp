"""Execute Python code in a sandboxed environment."""

from typing import Any

from mcp.server import Server
from mcp.types import TextContent, Tool
import structlog

from xatu_mcp.config import Config
from xatu_mcp.sandbox.base import SandboxBackend

logger = structlog.get_logger()


def register_execute_python(server: Server, sandbox: SandboxBackend, config: Config) -> None:
    """Register the execute_python tool with the MCP server.

    Args:
        server: The MCP server instance.
        sandbox: The sandbox backend to use for execution.
        config: Server configuration.
    """

    @server.list_tools()
    async def list_tools() -> list[Tool]:
        return [
            Tool(
                name="execute_python",
                description="""Execute Python code in a sandboxed environment.

The xatu library is pre-installed for querying Ethereum network data:

```python
from xatu import clickhouse, prometheus, loki, storage

# Query ClickHouse for blockchain data
df = clickhouse.query("mainnet", "SELECT * FROM beacon_api_eth_v1_events_block LIMIT 10")

# Query Prometheus metrics
result = prometheus.query("up")

# Generate and save charts
import matplotlib.pyplot as plt
plt.figure(figsize=(10, 6))
plt.plot(df['slot'], df['block_root'])
plt.savefig('/output/chart.png')

# Upload to get a URL
url = storage.upload('/output/chart.png')
print(f"Chart: {url}")
```

Available ClickHouse clusters:
- "xatu": Production raw data (mainnet, sepolia, holesky, hoodi)
- "xatu-experimental": Devnet raw data
- "xatu-cbt": Aggregated/CBT tables

All output files should be written to /output/ directory.
Data stays in the sandbox - Claude only sees stdout and file URLs.""",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "code": {
                            "type": "string",
                            "description": "Python code to execute",
                        },
                        "timeout": {
                            "type": "integer",
                            "description": "Execution timeout in seconds (default: 60, max: 300)",
                            "minimum": 1,
                            "maximum": 300,
                            "default": 60,
                        },
                    },
                    "required": ["code"],
                },
            ),
        ]

    @server.call_tool()
    async def call_tool(name: str, arguments: dict[str, Any]) -> list[TextContent]:
        if name != "execute_python":
            raise ValueError(f"Unknown tool: {name}")

        code = arguments.get("code")
        if not code:
            raise ValueError("Code is required")

        timeout = arguments.get("timeout", 60)
        timeout = min(max(1, timeout), 300)  # Clamp to 1-300 seconds

        logger.info(
            "Executing Python code",
            code_length=len(code),
            timeout=timeout,
            backend=sandbox.name,
        )

        # Build environment variables for the sandbox
        env = _build_sandbox_env(config)

        try:
            result = await sandbox.execute(code=code, env=env, timeout=timeout)
        except TimeoutError as e:
            logger.warning("Execution timed out", timeout=timeout)
            return [
                TextContent(
                    type="text",
                    text=f"Execution timed out after {timeout} seconds",
                )
            ]
        except Exception as e:
            logger.error("Execution failed", error=str(e))
            return [
                TextContent(
                    type="text",
                    text=f"Execution error: {e}",
                )
            ]

        # Build response
        response_parts = []

        if result.stdout:
            response_parts.append(f"=== STDOUT ===\n{result.stdout}")

        if result.stderr:
            response_parts.append(f"=== STDERR ===\n{result.stderr}")

        if result.output_files:
            files_list = "\n".join(f"  - {f}" for f in result.output_files)
            response_parts.append(f"=== OUTPUT FILES ===\n{files_list}")

        response_parts.append(f"=== EXIT CODE: {result.exit_code} ===")
        response_parts.append(f"=== DURATION: {result.duration_seconds:.2f}s ===")

        logger.info(
            "Execution completed",
            exit_code=result.exit_code,
            duration=result.duration_seconds,
            output_files=result.output_files,
        )

        return [
            TextContent(
                type="text",
                text="\n\n".join(response_parts),
            )
        ]


def _build_sandbox_env(config: Config) -> dict[str, str]:
    """Build environment variables to pass to the sandbox.

    Args:
        config: Server configuration.

    Returns:
        Dictionary of environment variables.
    """
    env: dict[str, str] = {}

    # ClickHouse clusters
    if config.clickhouse.xatu:
        env["XATU_CLICKHOUSE_HOST"] = config.clickhouse.xatu.host
        env["XATU_CLICKHOUSE_PORT"] = str(config.clickhouse.xatu.port)
        env["XATU_CLICKHOUSE_PROTOCOL"] = config.clickhouse.xatu.protocol
        env["XATU_CLICKHOUSE_USER"] = config.clickhouse.xatu.user
        env["XATU_CLICKHOUSE_PASSWORD"] = config.clickhouse.xatu.password
        env["XATU_CLICKHOUSE_DATABASE"] = config.clickhouse.xatu.database

    if config.clickhouse.xatu_experimental:
        env["XATU_EXPERIMENTAL_CLICKHOUSE_HOST"] = config.clickhouse.xatu_experimental.host
        env["XATU_EXPERIMENTAL_CLICKHOUSE_PORT"] = str(config.clickhouse.xatu_experimental.port)
        env["XATU_EXPERIMENTAL_CLICKHOUSE_PROTOCOL"] = config.clickhouse.xatu_experimental.protocol
        env["XATU_EXPERIMENTAL_CLICKHOUSE_USER"] = config.clickhouse.xatu_experimental.user
        env["XATU_EXPERIMENTAL_CLICKHOUSE_PASSWORD"] = config.clickhouse.xatu_experimental.password
        env["XATU_EXPERIMENTAL_CLICKHOUSE_DATABASE"] = config.clickhouse.xatu_experimental.database

    if config.clickhouse.xatu_cbt:
        env["XATU_CBT_CLICKHOUSE_HOST"] = config.clickhouse.xatu_cbt.host
        env["XATU_CBT_CLICKHOUSE_PORT"] = str(config.clickhouse.xatu_cbt.port)
        env["XATU_CBT_CLICKHOUSE_PROTOCOL"] = config.clickhouse.xatu_cbt.protocol
        env["XATU_CBT_CLICKHOUSE_USER"] = config.clickhouse.xatu_cbt.user
        env["XATU_CBT_CLICKHOUSE_PASSWORD"] = config.clickhouse.xatu_cbt.password
        env["XATU_CBT_CLICKHOUSE_DATABASE"] = config.clickhouse.xatu_cbt.database

    # Prometheus
    if config.prometheus:
        env["XATU_PROMETHEUS_URL"] = config.prometheus.url

    # Loki
    if config.loki:
        env["XATU_LOKI_URL"] = config.loki.url

    # S3 Storage
    if config.storage:
        env["XATU_S3_ENDPOINT"] = config.storage.endpoint
        env["XATU_S3_ACCESS_KEY"] = config.storage.access_key
        env["XATU_S3_SECRET_KEY"] = config.storage.secret_key
        env["XATU_S3_BUCKET"] = config.storage.bucket
        env["XATU_S3_REGION"] = config.storage.region
        if config.storage.public_url_prefix:
            env["XATU_S3_PUBLIC_URL_PREFIX"] = config.storage.public_url_prefix

    return env
