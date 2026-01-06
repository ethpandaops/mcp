"""ClickHouse data access for Ethereum blockchain data.

This module provides functions to query ClickHouse clusters containing
Ethereum network data from the Xatu project.

Available clusters:
- xatu: Production raw data (mainnet, sepolia, holesky, hoodi)
- xatu-experimental: Devnet raw data
- xatu-cbt: Aggregated/CBT tables

Example:
    from xatu import clickhouse

    # Query production data
    df = clickhouse.query("mainnet", "SELECT * FROM beacon_api_eth_v1_events_block LIMIT 10")

    # Query with explicit cluster
    df = clickhouse.query("mainnet", "SELECT * FROM cbt_slots", cluster="xatu-cbt")

    # List available tables
    tables = clickhouse.list_tables(cluster="xatu")

    # Get table schema
    schema = clickhouse.describe_table("beacon_api_eth_v1_events_block")
"""

import os
from typing import Any

import clickhouse_connect
import pandas as pd

# Cluster configuration from environment variables
_CLUSTERS = {
    "xatu": {
        "host": os.environ.get("XATU_CLICKHOUSE_HOST", ""),
        "port": int(os.environ.get("XATU_CLICKHOUSE_PORT", "443")),
        "protocol": os.environ.get("XATU_CLICKHOUSE_PROTOCOL", "https"),
        "user": os.environ.get("XATU_CLICKHOUSE_USER", ""),
        "password": os.environ.get("XATU_CLICKHOUSE_PASSWORD", ""),
        "database": os.environ.get("XATU_CLICKHOUSE_DATABASE", "default"),
        "networks": ["mainnet", "sepolia", "holesky", "hoodi"],
    },
    "xatu-experimental": {
        "host": os.environ.get("XATU_EXPERIMENTAL_CLICKHOUSE_HOST", ""),
        "port": int(os.environ.get("XATU_EXPERIMENTAL_CLICKHOUSE_PORT", "443")),
        "protocol": os.environ.get("XATU_EXPERIMENTAL_CLICKHOUSE_PROTOCOL", "https"),
        "user": os.environ.get("XATU_EXPERIMENTAL_CLICKHOUSE_USER", ""),
        "password": os.environ.get("XATU_EXPERIMENTAL_CLICKHOUSE_PASSWORD", ""),
        "database": os.environ.get("XATU_EXPERIMENTAL_CLICKHOUSE_DATABASE", "default"),
        "networks": [],  # Dynamic devnets
    },
    "xatu-cbt": {
        "host": os.environ.get("XATU_CBT_CLICKHOUSE_HOST", ""),
        "port": int(os.environ.get("XATU_CBT_CLICKHOUSE_PORT", "443")),
        "protocol": os.environ.get("XATU_CBT_CLICKHOUSE_PROTOCOL", "https"),
        "user": os.environ.get("XATU_CBT_CLICKHOUSE_USER", ""),
        "password": os.environ.get("XATU_CBT_CLICKHOUSE_PASSWORD", ""),
        "database": os.environ.get("XATU_CBT_CLICKHOUSE_DATABASE", "default"),
        "networks": ["mainnet", "sepolia", "holesky"],
    },
}

# Cache for clients
_clients: dict[str, clickhouse_connect.driver.Client] = {}


def _get_cluster_for_network(network: str) -> str:
    """Determine which cluster to use for a network.

    Args:
        network: Network name (e.g., "mainnet", "holesky").

    Returns:
        Cluster name.

    Raises:
        ValueError: If network is not found in any cluster.
    """
    for cluster_name, config in _CLUSTERS.items():
        if network in config["networks"]:
            return cluster_name

    # Default to xatu-experimental for unknown networks (devnets)
    return "xatu-experimental"


def _get_client(cluster: str) -> clickhouse_connect.driver.Client:
    """Get or create a ClickHouse client for a cluster.

    Args:
        cluster: Cluster name.

    Returns:
        ClickHouse client.

    Raises:
        ValueError: If cluster is unknown or not configured.
    """
    if cluster in _clients:
        return _clients[cluster]

    if cluster not in _CLUSTERS:
        raise ValueError(f"Unknown cluster: {cluster}. Available: {list(_CLUSTERS.keys())}")

    config = _CLUSTERS[cluster]

    if not config["host"]:
        raise ValueError(
            f"Cluster '{cluster}' not configured. "
            f"Set XATU{'_' + cluster.upper().replace('-', '_') if cluster != 'xatu' else ''}_CLICKHOUSE_HOST"
        )

    secure = config["protocol"] == "https"

    client = clickhouse_connect.get_client(
        host=config["host"],
        port=config["port"],
        username=config["user"],
        password=config["password"],
        database=config["database"],
        secure=secure,
    )

    _clients[cluster] = client
    return client


def query(
    network: str,
    sql: str,
    cluster: str = "auto",
    parameters: dict[str, Any] | None = None,
) -> pd.DataFrame:
    """Execute a SQL query and return results as a DataFrame.

    Args:
        network: Network name (e.g., "mainnet", "holesky"). Used for auto cluster selection.
        sql: SQL query to execute.
        cluster: Which cluster to query. Use "auto" to select based on network.
            - "xatu": Raw production data
            - "xatu-experimental": Raw devnet data
            - "xatu-cbt": Aggregated/CBT tables
            - "auto": Auto-select based on network (default)
        parameters: Query parameters for parameterized queries.

    Returns:
        DataFrame with query results.

    Example:
        >>> df = query("mainnet", "SELECT * FROM blocks LIMIT 10")
        >>> df = query("mainnet", "SELECT * FROM cbt_slots", cluster="xatu-cbt")
    """
    if cluster == "auto":
        cluster = _get_cluster_for_network(network)

    client = _get_client(cluster)

    result = client.query(sql, parameters=parameters)

    return result.result_set_to_df()


def query_raw(
    network: str,
    sql: str,
    cluster: str = "auto",
    parameters: dict[str, Any] | None = None,
) -> tuple[list[tuple], list[str]]:
    """Execute a SQL query and return raw results.

    Args:
        network: Network name.
        sql: SQL query to execute.
        cluster: Which cluster to query.
        parameters: Query parameters.

    Returns:
        Tuple of (rows, column_names).
    """
    if cluster == "auto":
        cluster = _get_cluster_for_network(network)

    client = _get_client(cluster)

    result = client.query(sql, parameters=parameters)

    return result.result_rows, result.column_names


def list_tables(cluster: str = "xatu") -> list[str]:
    """List all tables in a cluster.

    Args:
        cluster: Cluster to query.

    Returns:
        List of table names.
    """
    client = _get_client(cluster)

    result = client.query("SHOW TABLES")

    return [row[0] for row in result.result_rows]


def describe_table(table: str, cluster: str = "xatu") -> pd.DataFrame:
    """Get the schema of a table.

    Args:
        table: Table name.
        cluster: Cluster where the table exists.

    Returns:
        DataFrame with column information.
    """
    client = _get_client(cluster)

    result = client.query(f"DESCRIBE TABLE {table}")

    return result.result_set_to_df()


def get_available_networks() -> dict[str, list[str]]:
    """Get available networks for each cluster.

    Returns:
        Dictionary mapping cluster names to lists of network names.
    """
    return {name: config["networks"] for name, config in _CLUSTERS.items()}
