"""Xatu data access library for Ethereum network analytics.

This library provides access to Ethereum network data through:
- ClickHouse: Raw and aggregated blockchain data
- Prometheus: Infrastructure metrics
- Loki: Log data
- Storage: S3-compatible file storage for outputs

Example usage:
    from xatu import clickhouse, prometheus, loki, storage

    # Query ClickHouse
    df = clickhouse.query("mainnet", "SELECT * FROM blocks LIMIT 10")

    # Query Prometheus
    metrics = prometheus.query("up")

    # Upload output file
    url = storage.upload("/output/chart.png")
"""

from . import clickhouse, prometheus, loki, storage

__all__ = ["clickhouse", "prometheus", "loki", "storage"]
__version__ = "0.1.0"
