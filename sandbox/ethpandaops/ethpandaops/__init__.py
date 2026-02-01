"""ethpandaops data access library for Ethereum network analytics.

This library provides direct access to Ethereum network data:
- ClickHouse: Raw and aggregated blockchain data
- Prometheus: Infrastructure metrics
- Loki: Log data
- Dora: Beacon chain explorer
- Syncoor: Sync test orchestration and monitoring
- Storage: S3-compatible file storage for outputs

Use list_datasources() on each module to discover available datasources or
check the datasources://list MCP resource.

Example usage:
    from ethpandaops import clickhouse, prometheus, loki, dora, syncoor, storage

    # List available ClickHouse clusters
    clusters = clickhouse.list_datasources()
    cluster_name = clusters[0]['name']  # e.g., "xatu"

    # Query ClickHouse using cluster name
    df = clickhouse.query(cluster_name, "SELECT * FROM beacon_api_eth_v1_events_block LIMIT 10")

    # Query Prometheus using instance name
    result = prometheus.query("ethpandaops", "up")

    # Get sync tests from Syncoor
    tests = syncoor.list_tests("mainnet")

    # Upload output file
    url = storage.upload("/workspace/chart.png")
"""

from . import storage

# Plugin modules are assembled at Docker build time
# and can be imported as: from ethpandaops import clickhouse, prometheus, loki, dora, syncoor
__all__ = ["storage"]
__version__ = "0.1.0"


def __getattr__(name):
    """Lazy import for plugin modules (clickhouse, prometheus, loki, dora, syncoor)."""
    if name in ("clickhouse", "prometheus", "loki", "dora", "syncoor"):
        import importlib

        mod = importlib.import_module(f".{name}", __name__)
        globals()[name] = mod
        return mod
    raise AttributeError(f"module {__name__!r} has no attribute {name!r}")
