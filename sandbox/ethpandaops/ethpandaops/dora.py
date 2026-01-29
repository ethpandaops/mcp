"""Dora beacon chain explorer access.

This module provides functions to query the Dora beacon chain explorer API
and generate deep links to the Dora UI. Dora is a public service, so no
credentials are needed.

Network URLs are discovered from cartographoor and injected via environment
variables by the MCP server.

Example:
    from ethpandaops import dora

    # List available networks with Dora explorers
    networks = dora.list_networks()

    # Get network overview
    overview = dora.get_network_overview("holesky")
    print(f"Current epoch: {overview['current_epoch']}")

    # Generate a deep link to a validator
    link = dora.link_validator("holesky", "12345")
    print(f"View in Dora: {link}")
"""

import json
import os
from typing import Any

import httpx

# Network configuration (injected by MCP server from cartographoor).
_NETWORKS: dict[str, str] | None = None

# HTTP client timeout settings.
_TIMEOUT = httpx.Timeout(connect=5.0, read=30.0, write=10.0, pool=5.0)


def _load_networks() -> dict[str, str]:
    """Load network -> Dora URL mapping from environment variable."""
    global _NETWORKS

    if _NETWORKS is not None:
        return _NETWORKS

    raw = os.environ.get("ETHPANDAOPS_DORA_NETWORKS", "")
    if not raw:
        _NETWORKS = {}
        return _NETWORKS

    try:
        _NETWORKS = json.loads(raw)
    except json.JSONDecodeError as e:
        raise ValueError(f"Invalid ETHPANDAOPS_DORA_NETWORKS JSON: {e}") from e

    return _NETWORKS


def _get_network_url(network: str) -> str:
    """Get the Dora base URL for a network."""
    networks = _load_networks()

    if network not in networks:
        available = list(networks.keys())
        raise ValueError(
            f"Unknown network '{network}'. Available networks: {available}"
        )

    return networks[network]


def list_networks() -> list[dict[str, str]]:
    """List networks with Dora explorers.

    Returns:
        List of dicts with 'name' and 'dora_url' keys.

    Example:
        >>> networks = list_networks()
        >>> for net in networks:
        ...     print(f"{net['name']}: {net['dora_url']}")
    """
    networks = _load_networks()
    return [{"name": name, "dora_url": url} for name, url in sorted(networks.items())]


def get_base_url(network: str) -> str:
    """Get the Dora base URL for a network.

    Args:
        network: Network name (e.g., 'holesky', 'mainnet').

    Returns:
        Dora base URL string.

    Example:
        >>> url = get_base_url("holesky")
        >>> print(url)  # https://dora.holesky.ethpandaops.io
    """
    return _get_network_url(network)


def _api_request(network: str, path: str, params: dict[str, Any] | None = None) -> dict[str, Any]:
    """Make a request to the Dora API."""
    base_url = _get_network_url(network)
    url = f"{base_url}{path}"

    with httpx.Client(timeout=_TIMEOUT) as client:
        response = client.get(url, params=params)
        response.raise_for_status()

        return response.json()


def get_network_overview(network: str) -> dict[str, Any]:
    """Get network overview including current epoch, slot, and validator counts.

    Args:
        network: Network name (e.g., 'holesky', 'mainnet').

    Returns:
        Dict with current_epoch, current_slot, active_validator_count, etc.

    Example:
        >>> overview = get_network_overview("holesky")
        >>> print(f"Current epoch: {overview['current_epoch']}")
        >>> print(f"Active validators: {overview['active_validator_count']}")
    """
    data = _api_request(network, "/api/v1/epoch/head")

    epoch_data = data.get("data", {})

    # Calculate current slot (rough estimate: epoch * 32 + slot_in_epoch)
    epoch = epoch_data.get("epoch", 0)
    current_slot = epoch * 32

    result = {
        "current_epoch": epoch,
        "current_slot": current_slot,
        "finalized": epoch_data.get("finalized", False),
        "participation_rate": epoch_data.get("globalparticipationrate", 0.0),
    }

    # Add validator info if available
    validator_info = epoch_data.get("validatorinfo", {})
    if validator_info:
        result["active_validator_count"] = validator_info.get("active", 0)
        result["total_validator_count"] = validator_info.get("total", 0)
        result["pending_validator_count"] = validator_info.get("pending", 0)
        result["exited_validator_count"] = validator_info.get("exited", 0)

    return result


def get_validator(network: str, index_or_pubkey: str) -> dict[str, Any]:
    """Get validator details by index or public key.

    Args:
        network: Network name.
        index_or_pubkey: Validator index (e.g., '12345') or public key (0x...).

    Returns:
        Dict with status, balance, activation_epoch, etc.

    Example:
        >>> validator = get_validator("holesky", "12345")
        >>> print(f"Status: {validator['status']}")
        >>> print(f"Balance: {validator['balance']} gwei")
    """
    data = _api_request(network, f"/api/v1/validator/{index_or_pubkey}")
    return data.get("data", {})


def get_validators(
    network: str,
    status: str | None = None,
    limit: int = 100,
) -> list[dict[str, Any]]:
    """Get list of validators with optional status filter.

    Args:
        network: Network name.
        status: Filter by status: 'active', 'pending', 'exited', etc.
        limit: Maximum number of validators to return.

    Returns:
        List of validator dicts.

    Example:
        >>> validators = get_validators("holesky", status="active", limit=10)
        >>> for v in validators:
        ...     print(f"Validator {v['index']}: {v['status']}")
    """
    params: dict[str, Any] = {"limit": limit}
    if status:
        params["status"] = status

    data = _api_request(network, "/api/v1/validators", params)
    return data.get("data", [])


def get_slot(network: str, slot_or_hash: str) -> dict[str, Any]:
    """Get slot details by slot number or block hash.

    Args:
        network: Network name.
        slot_or_hash: Slot number or block hash.

    Returns:
        Dict with slot, proposer, status, etc.

    Example:
        >>> slot_info = get_slot("holesky", "1000000")
        >>> print(f"Slot {slot_info['slot']}: proposer {slot_info['proposer']}")
    """
    data = _api_request(network, f"/api/v1/slot/{slot_or_hash}")
    return data.get("data", {})


def get_epoch(network: str, epoch: int) -> dict[str, Any]:
    """Get epoch summary.

    Args:
        network: Network name.
        epoch: Epoch number.

    Returns:
        Dict with epoch stats and finalization status.

    Example:
        >>> epoch_info = get_epoch("holesky", 250000)
        >>> print(f"Epoch {epoch_info['epoch']}: finalized={epoch_info['finalized']}")
    """
    data = _api_request(network, f"/api/v1/epoch/{epoch}")
    return data.get("data", {})


# Deep link generation functions


def link_validator(network: str, index_or_pubkey: str) -> str:
    """Generate a Dora deep link to a validator.

    Args:
        network: Network name.
        index_or_pubkey: Validator index or public key.

    Returns:
        URL string to view validator in Dora.

    Example:
        >>> link = link_validator("holesky", "12345")
        >>> print(link)  # https://dora.holesky.ethpandaops.io/validator/12345
    """
    base_url = _get_network_url(network)
    return f"{base_url}/validator/{index_or_pubkey}"


def link_slot(network: str, slot_or_hash: str) -> str:
    """Generate a Dora deep link to a slot/block.

    Args:
        network: Network name.
        slot_or_hash: Slot number or block hash.

    Returns:
        URL string to view slot in Dora.

    Example:
        >>> link = link_slot("holesky", "1000000")
        >>> print(link)  # https://dora.holesky.ethpandaops.io/slot/1000000
    """
    base_url = _get_network_url(network)
    return f"{base_url}/slot/{slot_or_hash}"


def link_epoch(network: str, epoch: int) -> str:
    """Generate a Dora deep link to an epoch.

    Args:
        network: Network name.
        epoch: Epoch number.

    Returns:
        URL string to view epoch in Dora.

    Example:
        >>> link = link_epoch("holesky", 250000)
        >>> print(link)  # https://dora.holesky.ethpandaops.io/epoch/250000
    """
    base_url = _get_network_url(network)
    return f"{base_url}/epoch/{epoch}"


def link_address(network: str, address: str) -> str:
    """Generate a Dora deep link to an execution layer address.

    Args:
        network: Network name.
        address: Ethereum address (0x...).

    Returns:
        URL string to view address in Dora.

    Example:
        >>> link = link_address("holesky", "0x742d35Cc6634C0532925a3b844Bc9e7595f1c1")
        >>> print(link)
    """
    base_url = _get_network_url(network)
    return f"{base_url}/address/{address}"


def link_block(network: str, number_or_hash: str) -> str:
    """Generate a Dora deep link to an execution layer block.

    Args:
        network: Network name.
        number_or_hash: Block number or hash.

    Returns:
        URL string to view block in Dora.

    Example:
        >>> link = link_block("holesky", "1000000")
        >>> print(link)  # https://dora.holesky.ethpandaops.io/block/1000000
    """
    base_url = _get_network_url(network)
    return f"{base_url}/block/{number_or_hash}"
