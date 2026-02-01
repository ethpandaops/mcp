"""Syncoor sync test orchestration and monitoring access.

Query the Syncoor API for Ethereum client synchronization tests and generate deep links.
Network URLs are discovered from cartographoor via environment variables.

Example:
    from ethpandaops import syncoor

    networks = syncoor.list_networks()
    tests = syncoor.list_tests("mainnet")
    test = syncoor.get_test("mainnet", "run-id-123")
    link = syncoor.link_test("mainnet", "run-id-123")
"""

import json
import os
from typing import Any

import httpx

_NETWORKS: dict[str, str] | None = None
_TIMEOUT = httpx.Timeout(connect=5.0, read=30.0, write=10.0, pool=5.0)


def _load_networks() -> dict[str, str]:
    """Load network -> Syncoor URL mapping from environment."""
    global _NETWORKS
    if _NETWORKS is not None:
        return _NETWORKS

    raw = os.environ.get("ETHPANDAOPS_SYNCOOR_NETWORKS", "")
    if not raw:
        _NETWORKS = {}
        return _NETWORKS

    try:
        _NETWORKS = json.loads(raw)
    except json.JSONDecodeError as e:
        raise ValueError(f"Invalid ETHPANDAOPS_SYNCOOR_NETWORKS JSON: {e}") from e

    return _NETWORKS


def _get_url(network: str) -> str:
    """Get Syncoor base URL for a network."""
    networks = _load_networks()
    if network not in networks:
        raise ValueError(f"Unknown network '{network}'. Available: {list(networks.keys())}")
    return networks[network]


def _api_get(network: str, path: str, params: dict[str, Any] | None = None) -> dict[str, Any]:
    """Make GET request to Syncoor API."""
    url = f"{_get_url(network)}{path}"
    with httpx.Client(timeout=_TIMEOUT) as client:
        resp = client.get(url, params=params)
        resp.raise_for_status()
        return resp.json()


# Network discovery


def list_networks() -> list[dict[str, str]]:
    """List networks with Syncoor instances."""
    return [{"name": n, "syncoor_url": u} for n, u in sorted(_load_networks().items())]


def get_base_url(network: str) -> str:
    """Get Syncoor base URL for a network."""
    return _get_url(network)


# API queries


def list_tests(network: str, active_only: bool = False) -> list[dict[str, Any]]:
    """List sync tests for a network.
    
    Args:
        network: The network name (e.g., "mainnet", "sepolia")
        active_only: If True, return only running tests
    
    Returns:
        List of test summaries with run_id, status, client info, etc.
    """
    params = {}
    if active_only:
        params["active"] = "true"
    
    resp = _api_get(network, "/api/v1/tests", params)
    data = resp.get("data", {})
    return data.get("tests", [])


def get_test(network: str, run_id: str) -> dict[str, Any]:
    """Get detailed information about a specific test.
    
    Args:
        network: The network name
        run_id: The unique test run identifier
    
    Returns:
        Test details including client configs, progress history, etc.
    """
    return _api_get(network, f"/api/v1/tests/{run_id}").get("data", {})


def get_test_progress(network: str, run_id: str) -> dict[str, Any] | None:
    """Get current progress metrics for a test.
    
    Args:
        network: The network name
        run_id: The unique test run identifier
    
    Returns:
        Current progress metrics including sync percentage, peers, etc.
        Returns None if test not found or has no current metrics.
    """
    test = get_test(network, run_id)
    return test.get("current_metrics")


def get_client_statistics(network: str, run_id: str) -> dict[str, Any]:
    """Get client sync statistics for a test.
    
    Args:
        network: The network name
        run_id: The unique test run identifier
    
    Returns:
        Dictionary with 'exec' and 'cons' keys containing client statistics
        including disk usage, memory usage, CPU usage, and sync percentage.
    """
    test = get_test(network, run_id)
    
    result = {
        "exec": {},
        "cons": {},
    }
    
    # Get client configs
    el_config = test.get("el_client_config", {})
    cl_config = test.get("cl_client_config", {})
    
    # Get current metrics
    metrics = test.get("current_metrics", {})
    
    # Execution client stats
    if el_config:
        result["exec"]["type"] = el_config.get("type", "unknown")
        result["exec"]["image"] = el_config.get("image", "")
    
    if metrics:
        # Convert bytes to GB/MB for readability
        exec_disk = metrics.get("exec_disk_usage", 0)
        exec_mem = metrics.get("exec_memory_usage", 0)
        
        result["exec"]["disk_usage_gb"] = exec_disk / (1024**3) if exec_disk else 0
        result["exec"]["memory_usage_mb"] = exec_mem / (1024**2) if exec_mem else 0
        result["exec"]["cpu_percent"] = metrics.get("exec_cpu_usage_percent", 0)
        result["exec"]["peers"] = metrics.get("exec_peers", 0)
        result["exec"]["sync_percent"] = metrics.get("exec_sync_percent", 0)
        result["exec"]["block"] = metrics.get("block", 0)
        result["exec"]["version"] = metrics.get("exec_version", "")
    
    # Consensus client stats
    if cl_config:
        result["cons"]["type"] = cl_config.get("type", "unknown")
        result["cons"]["image"] = cl_config.get("image", "")
    
    if metrics:
        cons_disk = metrics.get("cons_disk_usage", 0)
        cons_mem = metrics.get("cons_memory_usage", 0)
        
        result["cons"]["disk_usage_gb"] = cons_disk / (1024**3) if cons_disk else 0
        result["cons"]["memory_usage_mb"] = cons_mem / (1024**2) if cons_mem else 0
        result["cons"]["cpu_percent"] = metrics.get("cons_cpu_usage_percent", 0)
        result["cons"]["peers"] = metrics.get("cons_peers", 0)
        result["cons"]["sync_percent"] = metrics.get("cons_sync_percent", 0)
        result["cons"]["slot"] = metrics.get("slot", 0)
        result["cons"]["version"] = metrics.get("cons_version", "")
    
    return result


# Deep links


def link_test(network: str, run_id: str) -> str:
    """Generate link to a specific test page.
    
    Args:
        network: The network name
        run_id: The unique test run identifier
    
    Returns:
        URL to view the test in the Syncoor web UI
    """
    return f"{_get_url(network)}/test/{run_id}"


def link_tests_list(network: str) -> str:
    """Generate link to the tests list page.
    
    Args:
        network: The network name
    
    Returns:
        URL to view all tests in the Syncoor web UI
    """
    return f"{_get_url(network)}/"
