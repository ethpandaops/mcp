"""Schema resources for ClickHouse cluster and table information."""

import json
import re

from mcp.server import Server
from mcp.types import Resource, ResourceTemplate
from pydantic import AnyUrl
import structlog

from mcp.server.lowlevel.helper_types import ReadResourceContents

from xatu_mcp.resources.clickhouse_client import (
    CLUSTERS,
    ClickHouseClient,
    get_cluster,
    list_clusters,
)

logger = structlog.get_logger()


def register_schema_resources(server: Server) -> None:
    """Register schema:// resources with the MCP server.

    Args:
        server: The MCP server instance.
    """

    @server.list_resources()
    async def list_resources() -> list[Resource]:
        """List available static schema resources."""
        return [
            Resource(
                uri=AnyUrl("schema://clusters"),
                name="ClickHouse Clusters",
                description="List of available ClickHouse clusters and their networks",
                mimeType="application/json",
            ),
        ]

    @server.list_resource_templates()
    async def list_resource_templates() -> list[ResourceTemplate]:
        """List available schema resource templates."""
        return [
            ResourceTemplate(
                uriTemplate="schema://tables/{cluster}",
                name="Cluster Tables",
                description="List all tables in a ClickHouse cluster. Cluster must be one of: xatu, xatu-experimental, xatu-cbt",
                mimeType="application/json",
            ),
            ResourceTemplate(
                uriTemplate="schema://tables/{cluster}/{table}",
                name="Table Schema",
                description="Detailed schema information for a specific table including columns, types, and keys",
                mimeType="application/json",
            ),
        ]

    @server.read_resource()
    async def read_resource(uri: AnyUrl) -> list[ReadResourceContents]:
        """Read a schema resource.

        Args:
            uri: The resource URI to read.

        Returns:
            The resource contents.

        Raises:
            ValueError: If the URI is not recognized or parameters are invalid.
        """
        uri_str = str(uri)
        logger.debug("Reading schema resource", uri=uri_str)

        # Handle schema://clusters
        if uri_str == "schema://clusters":
            return await _read_clusters()

        # Handle schema://tables/{cluster}
        tables_match = re.match(r"^schema://tables/([^/]+)$", uri_str)
        if tables_match:
            cluster_name = tables_match.group(1)
            return await _read_tables(cluster_name)

        # Handle schema://tables/{cluster}/{table}
        table_schema_match = re.match(r"^schema://tables/([^/]+)/([^/]+)$", uri_str)
        if table_schema_match:
            cluster_name = table_schema_match.group(1)
            table_name = table_schema_match.group(2)
            return await _read_table_schema(cluster_name, table_name)

        raise ValueError(f"Unknown schema resource URI: {uri_str}")


async def _read_clusters() -> list[ReadResourceContents]:
    """Read the clusters resource."""
    clusters_data = []
    for cluster in list_clusters():
        clusters_data.append({
            "name": cluster.name,
            "description": cluster.description,
            "host": cluster.host,
            "networks": cluster.networks,
        })

    content = json.dumps({"clusters": clusters_data}, indent=2)
    return [ReadResourceContents(content=content, mime_type="application/json")]


async def _read_tables(cluster_name: str) -> list[ReadResourceContents]:
    """Read the tables list for a cluster.

    Args:
        cluster_name: The cluster name.

    Returns:
        List of table information.

    Raises:
        ValueError: If the cluster is not found.
    """
    cluster = get_cluster(cluster_name)
    if cluster is None:
        available = ", ".join(CLUSTERS.keys())
        raise ValueError(f"Unknown cluster: {cluster_name}. Available clusters: {available}")

    client = ClickHouseClient(cluster)
    try:
        tables = await client.list_tables()

        # Format the table data
        tables_data = []
        for table in tables:
            tables_data.append({
                "name": table["name"],
                "engine": table["engine"],
                "total_rows": table.get("total_rows", "0"),
                "total_bytes": table.get("total_bytes", "0"),
                "comment": table.get("comment", ""),
            })

        content = json.dumps(
            {
                "cluster": cluster_name,
                "database": cluster.database,
                "table_count": len(tables_data),
                "tables": tables_data,
            },
            indent=2,
        )
        return [ReadResourceContents(content=content, mime_type="application/json")]
    finally:
        await client.close()


async def _read_table_schema(cluster_name: str, table_name: str) -> list[ReadResourceContents]:
    """Read the detailed schema for a specific table.

    Args:
        cluster_name: The cluster name.
        table_name: The table name.

    Returns:
        Detailed table schema information.

    Raises:
        ValueError: If the cluster or table is not found.
    """
    cluster = get_cluster(cluster_name)
    if cluster is None:
        available = ", ".join(CLUSTERS.keys())
        raise ValueError(f"Unknown cluster: {cluster_name}. Available clusters: {available}")

    client = ClickHouseClient(cluster)
    try:
        # Get table metadata
        table_info = await client.get_table_info(table_name)
        if table_info is None:
            raise ValueError(f"Table not found: {table_name} in cluster {cluster_name}")

        # Get column schema
        columns = await client.get_table_schema(table_name)

        # Format the schema data
        columns_data = []
        for col in columns:
            col_data = {
                "name": col["name"],
                "type": col["type"],
                "comment": col.get("comment", ""),
            }
            if col.get("default_kind"):
                col_data["default_kind"] = col["default_kind"]
                col_data["default_expression"] = col.get("default_expression", "")
            if col.get("is_in_partition_key") == "1":
                col_data["is_partition_key"] = True
            if col.get("is_in_sorting_key") == "1":
                col_data["is_sorting_key"] = True
            if col.get("is_in_primary_key") == "1":
                col_data["is_primary_key"] = True
            columns_data.append(col_data)

        content = json.dumps(
            {
                "cluster": cluster_name,
                "table": table_name,
                "engine": table_info.get("engine", ""),
                "total_rows": table_info.get("total_rows", "0"),
                "total_bytes": table_info.get("total_bytes", "0"),
                "comment": table_info.get("comment", ""),
                "partition_key": table_info.get("partition_key", ""),
                "sorting_key": table_info.get("sorting_key", ""),
                "primary_key": table_info.get("primary_key", ""),
                "columns": columns_data,
            },
            indent=2,
        )
        return [ReadResourceContents(content=content, mime_type="application/json")]
    finally:
        await client.close()
