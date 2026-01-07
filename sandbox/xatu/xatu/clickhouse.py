"""ClickHouse data access for Ethereum blockchain data.

This module provides functions to query ClickHouse clusters containing
Ethereum network data from the Xatu project.

Available clusters:
- xatu: Production raw data (mainnet, sepolia, holesky, hoodi)
- xatu-experimental: Devnet raw data
- xatu-cbt: Aggregated/CBT tables

Example:
    from xatu import clickhouse

    # Query production data (cluster must be explicitly specified)
    df = clickhouse.query("mainnet", "SELECT * FROM beacon_api_eth_v1_events_block LIMIT 10", cluster="xatu")

    # Query CBT tables
    df = clickhouse.query("mainnet", "SELECT * FROM cbt_slots", cluster="xatu-cbt")

    # List available tables
    tables = clickhouse.list_tables(cluster="xatu")

    # Get table schema
    schema = clickhouse.describe_table("beacon_api_eth_v1_events_block", cluster="xatu")
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
        "networks": ["mainnet", "sepolia", "holesky", "hoodi"],
    },
}

# Cache for clients
_clients: dict[str, clickhouse_connect.driver.Client] = {}


def _get_env_var_name(cluster: str, suffix: str) -> str:
    """Get the environment variable name for a cluster config.

    Args:
        cluster: Cluster name (e.g., "xatu", "xatu-experimental").
        suffix: Config suffix (e.g., "HOST", "PORT").

    Returns:
        Environment variable name.
    """
    if cluster == "xatu":
        return f"XATU_CLICKHOUSE_{suffix}"
    else:
        cluster_part = cluster.upper().replace("-", "_").replace("XATU_", "")
        return f"XATU_{cluster_part}_CLICKHOUSE_{suffix}"


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
        raise ValueError(f"Unknown cluster: '{cluster}'. Available: {list(_CLUSTERS.keys())}")

    config = _CLUSTERS[cluster]

    if not config["host"]:
        raise ValueError(
            f"Cluster '{cluster}' not configured. "
            f"Set {_get_env_var_name(cluster, 'HOST')} environment variable."
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
    cluster: str,
    parameters: dict[str, Any] | None = None,
) -> pd.DataFrame:
    """Execute a SQL query and return results as a DataFrame.

    Args:
        network: Network name (e.g., "mainnet", "holesky").
        sql: SQL query to execute.
        cluster: Which cluster to query. Must be explicitly specified.
            - "xatu": Raw production data
            - "xatu-experimental": Raw devnet data
            - "xatu-cbt": Aggregated/CBT tables
        parameters: Query parameters for parameterized queries.

    Returns:
        DataFrame with query results.

    Raises:
        ValueError: If cluster is not specified or is invalid.

    Example:
        >>> df = query("mainnet", "SELECT * FROM blocks LIMIT 10", cluster="xatu")
        >>> df = query("mainnet", "SELECT * FROM cbt_slots", cluster="xatu-cbt")
    """
    if not cluster:
        raise ValueError(
            f"cluster parameter is required. Available clusters: {list(_CLUSTERS.keys())}"
        )

    client = _get_client(cluster)

    return client.query_df(sql, parameters=parameters)


def query_raw(
    network: str,
    sql: str,
    cluster: str,
    parameters: dict[str, Any] | None = None,
) -> tuple[list[tuple], list[str]]:
    """Execute a SQL query and return raw results.

    Args:
        network: Network name.
        sql: SQL query to execute.
        cluster: Which cluster to query. Must be explicitly specified.
            - "xatu": Raw production data
            - "xatu-experimental": Raw devnet data
            - "xatu-cbt": Aggregated/CBT tables
        parameters: Query parameters.

    Returns:
        Tuple of (rows, column_names).

    Raises:
        ValueError: If cluster is not specified or is invalid.
    """
    if not cluster:
        raise ValueError(
            f"cluster parameter is required. Available clusters: {list(_CLUSTERS.keys())}"
        )

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

    return client.query_df(f"DESCRIBE TABLE {table}")


def get_available_networks() -> dict[str, list[str]]:
    """Get available networks for each cluster.

    Returns:
        Dictionary mapping cluster names to lists of network names.
    """
    return {name: config["networks"] for name, config in _CLUSTERS.items()}
