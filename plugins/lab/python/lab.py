"""Lab explorer access.

Query the Lab explorer and generate deep links using routes.json patterns.
Network URLs are discovered from cartographoor via environment variables.

Example:
    from ethpandaops import lab

    networks = lab.list_networks()
    routes = lab.get_routes()
    link = lab.link_slot("mainnet", 1000000)
"""

import json
import os
import re
from typing import Any

import httpx

_NETWORKS: dict[str, str] | None = None
_ROUTES: dict[str, Any] | None = None
_SKILL_PATTERNS: dict[str, Any] | None = None
_TIMEOUT = httpx.Timeout(connect=5.0, read=30.0, write=10.0, pool=5.0)

# Default Lab URLs for known networks (fallback if cartographoor doesn't have Lab)
_DEFAULT_LAB_URLS = {
    "mainnet": "https://lab.ethpandaops.io",
    "sepolia": "https://lab.ethpandaops.io/sepolia",
    "holesky": "https://lab.ethpandaops.io/holesky",
    "hoodi": "https://lab.ethpandaops.io/hoodi",
}


def _load_networks() -> dict[str, str]:
    """Load network -> Lab URL mapping from environment."""
    global _NETWORKS
    if _NETWORKS is not None:
        return _NETWORKS

    raw = os.environ.get("ETHPANDAOPS_LAB_NETWORKS", "")
    if raw:
        try:
            _NETWORKS = json.loads(raw)
        except json.JSONDecodeError as e:
            raise ValueError(f"Invalid ETHPANDAOPS_LAB_NETWORKS JSON: {e}") from e
    else:
        _NETWORKS = {}

    # Merge with defaults for any missing networks
    for network, url in _DEFAULT_LAB_URLS.items():
        if network not in _NETWORKS:
            _NETWORKS[network] = url

    return _NETWORKS


def _get_url(network: str) -> str:
    """Get Lab base URL for a network."""
    networks = _load_networks()
    if network not in networks:
        # Try to construct from default pattern
        if network in _DEFAULT_LAB_URLS:
            return _DEFAULT_LAB_URLS[network]
        raise ValueError(f"Unknown network '{network}'. Available: {list(networks.keys())}")
    return networks[network]


def _fetch_routes() -> dict[str, Any]:
    """Fetch routes.json from the configured URL."""
    global _ROUTES
    if _ROUTES is not None:
        return _ROUTES

    routes_url = os.environ.get(
        "ETHPANDAOPS_LAB_ROUTES_URL",
        "https://raw.githubusercontent.com/ethpandaops/lab/main/routes.json"
    )

    try:
        with httpx.Client(timeout=_TIMEOUT) as client:
            resp = client.get(routes_url)
            resp.raise_for_status()
            _ROUTES = resp.json()
    except Exception as e:
        # Return default routes structure if fetch fails
        _ROUTES = _get_default_routes()

    return _ROUTES


def _get_default_routes() -> dict[str, Any]:
    """Get default routes structure."""
    return {
        "ethereum": [
            {
                "id": "ethereum/slots",
                "path": "/ethereum/slots/{slot}",
                "description": "Ethereum slot details",
                "parameters": [{"name": "slot", "type": "integer", "required": True}],
            },
            {
                "id": "ethereum/epochs",
                "path": "/ethereum/epochs/{epoch}",
                "description": "Ethereum epoch details",
                "parameters": [{"name": "epoch", "type": "integer", "required": True}],
            },
            {
                "id": "ethereum/validators",
                "path": "/ethereum/validators/{validator}",
                "description": "Validator details by index or pubkey",
                "parameters": [{"name": "validator", "type": "string", "required": True}],
            },
            {
                "id": "ethereum/blocks",
                "path": "/ethereum/blocks/{block}",
                "description": "Execution layer block details",
                "parameters": [{"name": "block", "type": "string", "required": True}],
            },
            {
                "id": "ethereum/transactions",
                "path": "/ethereum/transactions/{tx_hash}",
                "description": "Transaction details",
                "parameters": [{"name": "tx_hash", "type": "string", "required": True}],
            },
            {
                "id": "ethereum/addresses",
                "path": "/ethereum/addresses/{address}",
                "description": "Address details",
                "parameters": [{"name": "address", "type": "string", "required": True}],
            },
            {
                "id": "ethereum/blobs",
                "path": "/ethereum/blobs/{blob_id}",
                "description": "Blob details",
                "parameters": [{"name": "blob_id", "type": "string", "required": True}],
            },
            {
                "id": "ethereum/forks",
                "path": "/ethereum/forks/{fork_name}",
                "description": "Fork information",
                "parameters": [{"name": "fork_name", "type": "string", "required": True}],
            },
        ]
    }


def _get_route_by_id(route_id: str) -> dict[str, Any] | None:
    """Get route metadata by ID."""
    routes = _fetch_routes()
    for category, route_list in routes.items():
        for route in route_list:
            if route.get("id") == route_id:
                return route
    return None


def _build_path(path_template: str, params: dict[str, Any]) -> str:
    """Build a path from a template and parameters."""
    result = path_template
    for key, value in params.items():
        result = result.replace(f"{{{key}}}", str(value))
    return result


# Network discovery


def list_networks() -> list[dict[str, str]]:
    """List networks with Lab explorers."""
    return [{"name": n, "lab_url": u} for n, u in sorted(_load_networks().items())]


def get_base_url(network: str) -> str:
    """Get Lab base URL for a network."""
    return _get_url(network)


# Routes API


def get_routes() -> dict[str, Any]:
    """Get all Lab routes from routes.json."""
    return _fetch_routes()


def get_routes_by_category(category: str) -> list[dict[str, Any]]:
    """Get routes filtered by category."""
    routes = _fetch_routes()
    return routes.get(category, [])


def get_route(route_id: str) -> dict[str, Any] | None:
    """Get route metadata by ID."""
    return _get_route_by_id(route_id)


# Deep links


def link_slot(network: str, slot: int) -> str:
    """Generate link to slot page."""
    base = _get_url(network)
    route = _get_route_by_id("ethereum/slots")
    if route:
        path = _build_path(route.get("path", "/ethereum/slots/{slot}"), {"slot": slot})
        return f"{base}{path}"
    return f"{base}/ethereum/slots/{slot}"


def link_epoch(network: str, epoch: int) -> str:
    """Generate link to epoch page."""
    base = _get_url(network)
    route = _get_route_by_id("ethereum/epochs")
    if route:
        path = _build_path(route.get("path", "/ethereum/epochs/{epoch}"), {"epoch": epoch})
        return f"{base}{path}"
    return f"{base}/ethereum/epochs/{epoch}"


def link_validator(network: str, index_or_pubkey: str) -> str:
    """Generate link to validator page."""
    base = _get_url(network)
    route = _get_route_by_id("ethereum/validators")
    if route:
        path = _build_path(route.get("path", "/ethereum/validators/{validator}"), {"validator": index_or_pubkey})
        return f"{base}{path}"
    return f"{base}/ethereum/validators/{index_or_pubkey}"


def link_block(network: str, number_or_hash: str) -> str:
    """Generate link to execution block page."""
    base = _get_url(network)
    route = _get_route_by_id("ethereum/blocks")
    if route:
        path = _build_path(route.get("path", "/ethereum/blocks/{block}"), {"block": number_or_hash})
        return f"{base}{path}"
    return f"{base}/ethereum/blocks/{number_or_hash}"


def link_address(network: str, address: str) -> str:
    """Generate link to address page."""
    base = _get_url(network)
    route = _get_route_by_id("ethereum/addresses")
    if route:
        path = _build_path(route.get("path", "/ethereum/addresses/{address}"), {"address": address})
        return f"{base}{path}"
    return f"{base}/ethereum/addresses/{address}"


def link_transaction(network: str, tx_hash: str) -> str:
    """Generate link to transaction page."""
    base = _get_url(network)
    route = _get_route_by_id("ethereum/transactions")
    if route:
        path = _build_path(route.get("path", "/ethereum/transactions/{tx_hash}"), {"tx_hash": tx_hash})
        return f"{base}{path}"
    return f"{base}/ethereum/transactions/{tx_hash}"


def link_blob(network: str, blob_id: str) -> str:
    """Generate link to blob page."""
    base = _get_url(network)
    route = _get_route_by_id("ethereum/blobs")
    if route:
        path = _build_path(route.get("path", "/ethereum/blobs/{blob_id}"), {"blob_id": blob_id})
        return f"{base}{path}"
    return f"{base}/ethereum/blobs/{blob_id}"


def link_fork(network: str, fork_name: str) -> str:
    """Generate link to fork information page."""
    base = _get_url(network)
    route = _get_route_by_id("ethereum/forks")
    if route:
        path = _build_path(route.get("path", "/ethereum/forks/{fork_name}"), {"fork_name": fork_name})
        return f"{base}{path}"
    return f"{base}/ethereum/forks/{fork_name}"


def build_url(network: str, route_id: str, params: dict[str, Any]) -> str:
    """Build URL for a specific route with parameters.
    
    Args:
        network: Network name (e.g., "mainnet", "sepolia")
        route_id: Route identifier (e.g., "ethereum/slots")
        params: Dictionary of parameters to substitute in the route path
        
    Returns:
        Complete URL for the route
        
    Raises:
        ValueError: If network or route is not found
    """
    base = _get_url(network)
    route = _get_route_by_id(route_id)
    
    if not route:
        raise ValueError(f"Unknown route '{route_id}'")
    
    path_template = route.get("path", "")
    if not path_template:
        raise ValueError(f"Route '{route_id}' has no path defined")
    
    path = _build_path(path_template, params)
    return f"{base}{path}"
