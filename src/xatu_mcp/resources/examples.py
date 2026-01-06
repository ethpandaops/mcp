"""Example query resources for common use cases."""

import json

from mcp.server import Server
from mcp.server.lowlevel.helper_types import ReadResourceContents
from mcp.types import Resource
from pydantic import AnyUrl
import structlog

logger = structlog.get_logger()

# Common query patterns organized by use case
QUERY_EXAMPLES = {
    "block_events": {
        "name": "Block Events",
        "description": "Queries for beacon block events and propagation",
        "examples": [
            {
                "name": "Recent blocks by network",
                "description": "Get the most recent blocks for a specific network",
                "query": """
SELECT
    slot,
    block_root,
    proposer_index,
    meta_network_name,
    slot_start_date_time
FROM beacon_api_eth_v1_events_block
WHERE meta_network_name = 'mainnet'
ORDER BY slot DESC
LIMIT 100
""".strip(),
                "cluster": "xatu",
            },
            {
                "name": "Block propagation times",
                "description": "Analyze block propagation delay across sentries",
                "query": """
SELECT
    slot,
    block_root,
    meta_client_name,
    propagation_slot_start_diff / 1000 as propagation_ms
FROM beacon_api_eth_v1_events_block
WHERE meta_network_name = 'mainnet'
  AND slot_start_date_time >= now() - INTERVAL 1 HOUR
ORDER BY slot DESC, propagation_ms ASC
LIMIT 1000
""".strip(),
                "cluster": "xatu",
            },
        ],
    },
    "attestations": {
        "name": "Attestations",
        "description": "Queries for attestation data and analysis",
        "examples": [
            {
                "name": "Attestation inclusion delay",
                "description": "Calculate average attestation inclusion delay by slot",
                "query": """
SELECT
    slot,
    avg(inclusion_delay) as avg_inclusion_delay,
    count() as attestation_count
FROM beacon_api_eth_v1_events_attestation
WHERE meta_network_name = 'mainnet'
  AND slot_start_date_time >= now() - INTERVAL 1 HOUR
GROUP BY slot
ORDER BY slot DESC
LIMIT 100
""".strip(),
                "cluster": "xatu",
            },
            {
                "name": "Committee attestation performance",
                "description": "Attestation performance by committee",
                "query": """
SELECT
    slot,
    committee_index,
    count() as attestation_count,
    uniqExact(aggregation_bits) as unique_aggregations
FROM beacon_api_eth_v1_events_attestation
WHERE meta_network_name = 'mainnet'
  AND slot_start_date_time >= now() - INTERVAL 1 HOUR
GROUP BY slot, committee_index
ORDER BY slot DESC, committee_index
LIMIT 500
""".strip(),
                "cluster": "xatu",
            },
        ],
    },
    "validators": {
        "name": "Validators",
        "description": "Queries for validator state and performance",
        "examples": [
            {
                "name": "Validator balance changes",
                "description": "Track validator balance changes over epochs",
                "query": """
SELECT
    epoch,
    validator_index,
    balance / 1e9 as balance_eth,
    effective_balance / 1e9 as effective_balance_eth
FROM beacon_api_eth_v1_beacon_states_validators
WHERE meta_network_name = 'mainnet'
  AND validator_index = 12345
ORDER BY epoch DESC
LIMIT 100
""".strip(),
                "cluster": "xatu",
            },
            {
                "name": "Active validator count by epoch",
                "description": "Count active validators per epoch",
                "query": """
SELECT
    epoch,
    countIf(status = 'active_ongoing') as active_validators
FROM beacon_api_eth_v1_beacon_states_validators
WHERE meta_network_name = 'mainnet'
  AND epoch >= toUInt64((toUnixTimestamp(now()) - 1606824000) / 384) - 100
GROUP BY epoch
ORDER BY epoch DESC
LIMIT 100
""".strip(),
                "cluster": "xatu",
            },
        ],
    },
    "consensus_timing": {
        "name": "Consensus Timing (CBT)",
        "description": "Queries for consensus block timing analysis using aggregated tables",
        "examples": [
            {
                "name": "Block timing distribution",
                "description": "Analyze block arrival times relative to slot start",
                "query": """
SELECT
    slot,
    block_seen_p50_ms,
    block_seen_p90_ms,
    block_seen_p99_ms,
    block_first_seen_ms,
    proposer_index
FROM cbt_block_timing
WHERE network = 'mainnet'
ORDER BY slot DESC
LIMIT 100
""".strip(),
                "cluster": "xatu-cbt",
            },
            {
                "name": "Attestation timing percentiles",
                "description": "Get attestation timing percentiles by slot",
                "query": """
SELECT
    slot,
    attestation_seen_p50_ms,
    attestation_seen_p90_ms,
    coverage_at_4s,
    coverage_at_8s
FROM cbt_attestation_timing
WHERE network = 'mainnet'
ORDER BY slot DESC
LIMIT 100
""".strip(),
                "cluster": "xatu-cbt",
            },
        ],
    },
    "blobs": {
        "name": "Blob Data (Post-Dencun)",
        "description": "Queries for EIP-4844 blob sidecar data",
        "examples": [
            {
                "name": "Recent blob sidecars",
                "description": "Get recent blob sidecars with their propagation times",
                "query": """
SELECT
    slot,
    block_root,
    blob_index,
    kzg_commitment,
    propagation_slot_start_diff / 1000 as propagation_ms
FROM beacon_api_eth_v1_events_blob_sidecar
WHERE meta_network_name = 'mainnet'
ORDER BY slot DESC, blob_index
LIMIT 100
""".strip(),
                "cluster": "xatu",
            },
            {
                "name": "Blob count per block",
                "description": "Count blobs per block over recent slots",
                "query": """
SELECT
    slot,
    block_root,
    count() as blob_count,
    avg(propagation_slot_start_diff) / 1000 as avg_propagation_ms
FROM beacon_api_eth_v1_events_blob_sidecar
WHERE meta_network_name = 'mainnet'
  AND slot_start_date_time >= now() - INTERVAL 1 HOUR
GROUP BY slot, block_root
ORDER BY slot DESC
LIMIT 100
""".strip(),
                "cluster": "xatu",
            },
        ],
    },
    "mempool": {
        "name": "Mempool/Transaction Pool",
        "description": "Queries for mempool and pending transaction data",
        "examples": [
            {
                "name": "Recent mempool transactions",
                "description": "Get recent pending transactions from the mempool",
                "query": """
SELECT
    hash,
    from_address,
    to_address,
    value / 1e18 as value_eth,
    gas,
    gas_price / 1e9 as gas_price_gwei,
    meta_client_name
FROM mempool_transaction
WHERE meta_network_name = 'mainnet'
ORDER BY event_date_time DESC
LIMIT 100
""".strip(),
                "cluster": "xatu",
            },
        ],
    },
    "network_analysis": {
        "name": "Network Analysis",
        "description": "Queries for analyzing network health and client diversity",
        "examples": [
            {
                "name": "Client diversity by blocks",
                "description": "Analyze which clients are proposing blocks",
                "query": """
SELECT
    meta_client_name,
    meta_client_version,
    count() as block_count
FROM beacon_api_eth_v1_events_block
WHERE meta_network_name = 'mainnet'
  AND slot_start_date_time >= now() - INTERVAL 24 HOUR
GROUP BY meta_client_name, meta_client_version
ORDER BY block_count DESC
LIMIT 50
""".strip(),
                "cluster": "xatu",
            },
            {
                "name": "Geographic distribution",
                "description": "Analyze geographic distribution of sentries",
                "query": """
SELECT
    meta_client_geo_country,
    meta_client_geo_city,
    count() as event_count,
    uniqExact(meta_client_name) as unique_clients
FROM beacon_api_eth_v1_events_block
WHERE meta_network_name = 'mainnet'
  AND slot_start_date_time >= now() - INTERVAL 1 HOUR
GROUP BY meta_client_geo_country, meta_client_geo_city
ORDER BY event_count DESC
LIMIT 50
""".strip(),
                "cluster": "xatu",
            },
        ],
    },
}


def register_examples_resources(server: Server) -> None:
    """Register examples:// resources with the MCP server.

    Note: This extends the existing list_resources handler, so it should be
    called after register_schema_resources.

    Args:
        server: The MCP server instance.
    """
    # We need to add to existing handlers instead of replacing them
    # Store reference to any existing handlers
    existing_list_resources = server.request_handlers.get("resources/list")
    existing_read_resource = server.request_handlers.get("resources/read")

    # The examples resource is a static resource
    examples_resource = Resource(
        uri=AnyUrl("examples://queries"),
        name="Query Examples",
        description="Common ClickHouse query patterns organized by use case",
        mimeType="application/json",
    )

    # Store this for the combined handler
    _examples_resources = [examples_resource]

    async def read_examples_resource(uri: AnyUrl) -> list[ReadResourceContents] | None:
        """Read examples resource if URI matches.

        Args:
            uri: The resource URI.

        Returns:
            Resource contents if this is an examples URI, None otherwise.
        """
        uri_str = str(uri)

        if uri_str == "examples://queries":
            content = json.dumps(
                {
                    "description": "Common ClickHouse query patterns for Xatu data analysis",
                    "categories": QUERY_EXAMPLES,
                },
                indent=2,
            )
            return [ReadResourceContents(content=content, mime_type="application/json")]

        return None

    # Export these for the combined handler in __init__.py
    register_examples_resources._resources = _examples_resources  # type: ignore[attr-defined]
    register_examples_resources._read_handler = read_examples_resource  # type: ignore[attr-defined]
