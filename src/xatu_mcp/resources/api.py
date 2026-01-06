"""API documentation resource for the xatu Python library."""

import json

from mcp.server import Server
from mcp.server.lowlevel.helper_types import ReadResourceContents
from mcp.types import Resource
from pydantic import AnyUrl
import structlog

logger = structlog.get_logger()

# API documentation for the xatu library available in the sandbox
XATU_API_DOCS = {
    "overview": {
        "description": "The xatu library provides easy access to Ethereum network data through ClickHouse, Prometheus, and Loki. It is pre-installed in the sandbox environment.",
        "import": "from xatu import clickhouse, prometheus, loki, storage",
    },
    "modules": {
        "clickhouse": {
            "description": "Query ClickHouse databases for Ethereum blockchain data",
            "functions": {
                "query": {
                    "signature": "clickhouse.query(cluster: str, sql: str) -> pandas.DataFrame",
                    "description": "Execute a SQL query against a ClickHouse cluster and return results as a pandas DataFrame",
                    "parameters": {
                        "cluster": "The cluster name: 'xatu', 'xatu-experimental', or 'xatu-cbt'",
                        "sql": "The SQL query to execute",
                    },
                    "returns": "pandas.DataFrame with query results",
                    "example": """
import pandas as pd
from xatu import clickhouse

# Query recent blocks
df = clickhouse.query("xatu", '''
    SELECT slot, block_root, proposer_index
    FROM beacon_api_eth_v1_events_block
    WHERE meta_network_name = 'mainnet'
    ORDER BY slot DESC
    LIMIT 100
''')

print(df.head())
""".strip(),
                },
                "query_iter": {
                    "signature": "clickhouse.query_iter(cluster: str, sql: str, batch_size: int = 10000) -> Iterator[pandas.DataFrame]",
                    "description": "Execute a query and return results in batches for memory-efficient processing of large datasets",
                    "parameters": {
                        "cluster": "The cluster name",
                        "sql": "The SQL query to execute",
                        "batch_size": "Number of rows per batch (default: 10000)",
                    },
                    "returns": "Iterator of pandas DataFrames",
                    "example": """
from xatu import clickhouse

# Process large dataset in batches
for batch_df in clickhouse.query_iter("xatu", "SELECT * FROM large_table", batch_size=5000):
    process(batch_df)
""".strip(),
                },
                "get_clusters": {
                    "signature": "clickhouse.get_clusters() -> dict[str, ClusterInfo]",
                    "description": "Get information about available ClickHouse clusters",
                    "returns": "Dictionary of cluster names to their configurations",
                },
            },
        },
        "prometheus": {
            "description": "Query Prometheus metrics for monitoring data",
            "functions": {
                "query": {
                    "signature": "prometheus.query(promql: str, time: datetime | None = None) -> dict",
                    "description": "Execute an instant PromQL query",
                    "parameters": {
                        "promql": "The PromQL query string",
                        "time": "Optional timestamp for the query (default: now)",
                    },
                    "returns": "Dictionary with query results",
                    "example": """
from xatu import prometheus

# Query current value
result = prometheus.query("up")
print(result)
""".strip(),
                },
                "query_range": {
                    "signature": "prometheus.query_range(promql: str, start: datetime, end: datetime, step: str = '1m') -> dict",
                    "description": "Execute a range PromQL query",
                    "parameters": {
                        "promql": "The PromQL query string",
                        "start": "Start time for the range",
                        "end": "End time for the range",
                        "step": "Query resolution step (e.g., '1m', '5m', '1h')",
                    },
                    "returns": "Dictionary with time series data",
                    "example": """
from datetime import datetime, timedelta
from xatu import prometheus

end = datetime.now()
start = end - timedelta(hours=1)
result = prometheus.query_range("rate(http_requests_total[5m])", start, end, "1m")
""".strip(),
                },
            },
        },
        "loki": {
            "description": "Query Loki for log data",
            "functions": {
                "query": {
                    "signature": "loki.query(logql: str, limit: int = 1000, start: datetime | None = None, end: datetime | None = None) -> list[dict]",
                    "description": "Execute a LogQL query",
                    "parameters": {
                        "logql": "The LogQL query string",
                        "limit": "Maximum number of log entries to return",
                        "start": "Optional start time",
                        "end": "Optional end time",
                    },
                    "returns": "List of log entries",
                    "example": """
from xatu import loki

# Query recent logs
logs = loki.query('{app="xatu"} |= "error"', limit=100)
for log in logs:
    print(log['timestamp'], log['message'])
""".strip(),
                },
            },
        },
        "storage": {
            "description": "Upload files to S3-compatible storage and get public URLs",
            "functions": {
                "upload": {
                    "signature": "storage.upload(file_path: str, content_type: str | None = None) -> str",
                    "description": "Upload a file and return its public URL",
                    "parameters": {
                        "file_path": "Path to the file to upload (should be in /output/)",
                        "content_type": "Optional MIME type (auto-detected if not provided)",
                    },
                    "returns": "Public URL of the uploaded file",
                    "example": """
import matplotlib.pyplot as plt
from xatu import storage

# Create a chart
plt.figure(figsize=(10, 6))
plt.plot([1, 2, 3], [1, 4, 9])
plt.title("Example Chart")
plt.savefig('/output/chart.png')
plt.close()

# Upload and get URL
url = storage.upload('/output/chart.png')
print(f"Chart available at: {url}")
""".strip(),
                },
                "upload_dataframe": {
                    "signature": "storage.upload_dataframe(df: pandas.DataFrame, filename: str, format: str = 'csv') -> str",
                    "description": "Upload a pandas DataFrame as a file and return its public URL",
                    "parameters": {
                        "df": "The DataFrame to upload",
                        "filename": "The filename to use (without path)",
                        "format": "Output format: 'csv', 'parquet', or 'json'",
                    },
                    "returns": "Public URL of the uploaded file",
                    "example": """
import pandas as pd
from xatu import clickhouse, storage

# Query data
df = clickhouse.query("xatu", "SELECT * FROM my_table LIMIT 1000")

# Upload as CSV
url = storage.upload_dataframe(df, "results.csv", format="csv")
print(f"Data available at: {url}")
""".strip(),
                },
            },
        },
    },
    "common_patterns": {
        "visualization": {
            "description": "Creating and uploading visualizations",
            "example": """
import matplotlib.pyplot as plt
import pandas as pd
from xatu import clickhouse, storage

# Query block timing data
df = clickhouse.query("xatu", '''
    SELECT
        toStartOfHour(slot_start_date_time) as hour,
        avg(propagation_slot_start_diff) / 1000 as avg_propagation_ms
    FROM beacon_api_eth_v1_events_block
    WHERE meta_network_name = 'mainnet'
      AND slot_start_date_time >= now() - INTERVAL 24 HOUR
    GROUP BY hour
    ORDER BY hour
''')

# Create visualization
plt.figure(figsize=(12, 6))
plt.plot(df['hour'], df['avg_propagation_ms'])
plt.xlabel('Time')
plt.ylabel('Average Propagation (ms)')
plt.title('Block Propagation Times - Last 24 Hours')
plt.xticks(rotation=45)
plt.tight_layout()
plt.savefig('/output/propagation.png', dpi=150)
plt.close()

url = storage.upload('/output/propagation.png')
print(f"Chart: {url}")
""".strip(),
        },
        "data_export": {
            "description": "Exporting query results for further analysis",
            "example": """
from xatu import clickhouse, storage

# Query and export large dataset
df = clickhouse.query("xatu", '''
    SELECT *
    FROM beacon_api_eth_v1_events_block
    WHERE meta_network_name = 'mainnet'
      AND slot_start_date_time >= now() - INTERVAL 1 HOUR
''')

# Export as Parquet for efficient storage
url = storage.upload_dataframe(df, "blocks_export.parquet", format="parquet")
print(f"Data export: {url}")
""".strip(),
        },
        "multi_cluster_analysis": {
            "description": "Comparing data across clusters",
            "example": """
from xatu import clickhouse

# Query raw data from xatu
raw_df = clickhouse.query("xatu", '''
    SELECT slot, count() as raw_count
    FROM beacon_api_eth_v1_events_block
    WHERE meta_network_name = 'mainnet'
      AND slot >= 10000000
    GROUP BY slot
    ORDER BY slot DESC
    LIMIT 100
''')

# Query aggregated data from xatu-cbt
cbt_df = clickhouse.query("xatu-cbt", '''
    SELECT slot, block_seen_p50_ms
    FROM cbt_block_timing
    WHERE network = 'mainnet'
      AND slot >= 10000000
    ORDER BY slot DESC
    LIMIT 100
''')

# Merge and analyze
merged = raw_df.merge(cbt_df, on='slot')
print(merged.head())
""".strip(),
        },
    },
    "best_practices": [
        "Always use LIMIT clauses to avoid fetching too much data",
        "Use time-based filters (slot_start_date_time) to limit query scope",
        "Prefer aggregations over fetching raw data when possible",
        "Use the appropriate cluster: xatu for raw data, xatu-cbt for aggregated timing data",
        "Write output files to /output/ directory before uploading",
        "Close matplotlib figures after saving to free memory",
        "Use query_iter for large datasets to avoid memory issues",
    ],
}


def register_api_resources(server: Server) -> None:
    """Register api:// resources with the MCP server.

    Args:
        server: The MCP server instance.
    """
    api_resource = Resource(
        uri=AnyUrl("api://xatu"),
        name="Xatu Library API",
        description="API documentation for the xatu Python library available in the sandbox",
        mimeType="application/json",
    )

    _api_resources = [api_resource]

    async def read_api_resource(uri: AnyUrl) -> list[ReadResourceContents] | None:
        """Read API resource if URI matches.

        Args:
            uri: The resource URI.

        Returns:
            Resource contents if this is an api URI, None otherwise.
        """
        uri_str = str(uri)

        if uri_str == "api://xatu":
            content = json.dumps(XATU_API_DOCS, indent=2)
            return [ReadResourceContents(content=content, mime_type="application/json")]

        return None

    # Export these for the combined handler
    register_api_resources._resources = _api_resources  # type: ignore[attr-defined]
    register_api_resources._read_handler = read_api_resource  # type: ignore[attr-defined]
