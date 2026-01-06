"""Network resources providing information about available Ethereum networks."""

import json

from mcp.server import Server
from mcp.server.lowlevel.helper_types import ReadResourceContents
from mcp.types import Resource
from pydantic import AnyUrl
import structlog

logger = structlog.get_logger()

# Network information with their characteristics and which cluster they're on
NETWORKS = {
    "mainnet": {
        "name": "mainnet",
        "display_name": "Ethereum Mainnet",
        "chain_id": 1,
        "description": "The main Ethereum production network",
        "clusters": ["xatu", "xatu-cbt"],
        "genesis_time": 1606824023,
        "slot_duration_seconds": 12,
        "slots_per_epoch": 32,
        "is_testnet": False,
        "is_devnet": False,
        "beacon_genesis_time": 1606824023,
    },
    "sepolia": {
        "name": "sepolia",
        "display_name": "Sepolia Testnet",
        "chain_id": 11155111,
        "description": "A permissioned testnet for application developers",
        "clusters": ["xatu", "xatu-cbt"],
        "genesis_time": 1655733600,
        "slot_duration_seconds": 12,
        "slots_per_epoch": 32,
        "is_testnet": True,
        "is_devnet": False,
        "beacon_genesis_time": 1655733600,
    },
    "holesky": {
        "name": "holesky",
        "display_name": "Holesky Testnet",
        "chain_id": 17000,
        "description": "A public testnet for staking, infrastructure, and protocol development",
        "clusters": ["xatu", "xatu-cbt"],
        "genesis_time": 1695902400,
        "slot_duration_seconds": 12,
        "slots_per_epoch": 32,
        "is_testnet": True,
        "is_devnet": False,
        "beacon_genesis_time": 1695902400,
    },
    "hoodi": {
        "name": "hoodi",
        "display_name": "Hoodi Testnet",
        "chain_id": 560048,
        "description": "A testnet for Pectra testing",
        "clusters": ["xatu", "xatu-cbt"],
        "genesis_time": 1742212800,
        "slot_duration_seconds": 12,
        "slots_per_epoch": 32,
        "is_testnet": True,
        "is_devnet": False,
        "beacon_genesis_time": 1742212800,
    },
}

# Cluster to network mapping
CLUSTER_NETWORKS = {
    "xatu": ["mainnet", "sepolia", "holesky", "hoodi"],
    "xatu-experimental": ["devnets"],
    "xatu-cbt": ["mainnet", "sepolia", "holesky", "hoodi"],
}


def register_networks_resources(server: Server) -> None:
    """Register networks:// resources with the MCP server.

    Args:
        server: The MCP server instance.
    """
    networks_resource = Resource(
        uri=AnyUrl("networks://available"),
        name="Available Networks",
        description="List of available Ethereum networks and their configurations",
        mimeType="application/json",
    )

    _networks_resources = [networks_resource]

    async def read_networks_resource(uri: AnyUrl) -> list[ReadResourceContents] | None:
        """Read networks resource if URI matches.

        Args:
            uri: The resource URI.

        Returns:
            Resource contents if this is a networks URI, None otherwise.
        """
        uri_str = str(uri)

        if uri_str == "networks://available":
            content = json.dumps(
                {
                    "description": "Available Ethereum networks with their configurations and cluster mappings",
                    "networks": NETWORKS,
                    "cluster_networks": CLUSTER_NETWORKS,
                    "usage_notes": {
                        "querying": "Use meta_network_name = '<network>' in WHERE clauses to filter by network",
                        "mainnet": "Use 'mainnet' for production Ethereum data",
                        "testnets": "Sepolia and Holesky are the primary testnets for most use cases",
                        "devnets": "Devnet data is only available on xatu-experimental cluster",
                    },
                },
                indent=2,
            )
            return [ReadResourceContents(content=content, mime_type="application/json")]

        return None

    # Export these for the combined handler
    register_networks_resources._resources = _networks_resources  # type: ignore[attr-defined]
    register_networks_resources._read_handler = read_networks_resource  # type: ignore[attr-defined]
