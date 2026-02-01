"""Spamoor transaction spammer access.

Interact with Spamoor daemon REST API to control and monitor
transaction spammers. Spamoor is a powerful Ethereum transaction
generator for testnets.

Example:
    from ethpandaops import spamoor

    # List all spammer instances
    instances = spamoor.list_instances("http://localhost:8080")

    # Start/stop spammers
    spamoor.start_instance("http://localhost:8080", 1)
    spamoor.stop_instance("http://localhost:8080", 1)

    # Get metrics
    metrics = spamoor.get_metrics("http://localhost:8080")

    # Generate deep links
    link = spamoor.link_instance("http://localhost:8080", 1)
"""

from typing import Any

import httpx

_TIMEOUT = httpx.Timeout(connect=5.0, read=30.0, write=10.0, pool=5.0)


def _api_get(base_url: str, path: str, params: dict[str, Any] | None = None) -> dict[str, Any]:
    """Make GET request to Spamoor API."""
    url = f"{base_url.rstrip('/')}{path}"
    with httpx.Client(timeout=_TIMEOUT) as client:
        resp = client.get(url, params=params)
        resp.raise_for_status()
        return resp.json()


def _api_post(base_url: str, path: str, json_data: dict[str, Any] | None = None) -> dict[str, Any]:
    """Make POST request to Spamoor API."""
    url = f"{base_url.rstrip('/')}{path}"
    with httpx.Client(timeout=_TIMEOUT) as client:
        resp = client.post(url, json=json_data)
        resp.raise_for_status()
        if resp.text:
            try:
                return resp.json()
            except Exception:
                return {"success": True, "response": resp.text}
        return {"success": True}


def _api_put(base_url: str, path: str, json_data: dict[str, Any] | None = None) -> dict[str, Any]:
    """Make PUT request to Spamoor API."""
    url = f"{base_url.rstrip('/')}{path}"
    with httpx.Client(timeout=_TIMEOUT) as client:
        resp = client.put(url, json=json_data)
        resp.raise_for_status()
        if resp.text:
            try:
                return resp.json()
            except Exception:
                return {"success": True, "response": resp.text}
        return {"success": True}


# Spammer instance management


def list_instances(base_url: str) -> list[dict[str, Any]]:
    """List all spammer instances.

    Args:
        base_url: Spamoor daemon base URL (e.g., "http://localhost:8080")

    Returns:
        List of spammer instance dictionaries with id, name, description,
        scenario, status, and created_at fields.

    Example:
        >>> instances = list_instances("http://localhost:8080")
        >>> for inst in instances:
        ...     print(f"{inst['id']}: {inst['name']} ({inst['scenario']})")
    """
    return _api_get(base_url, "/api/spammers")


def get_instance(base_url: str, instance_id: int) -> dict[str, Any]:
    """Get spammer instance details.

    Args:
        base_url: Spamoor daemon base URL
        instance_id: Spammer instance ID

    Returns:
        Spammer details dictionary with id, name, description,
        scenario, config, and status fields.

    Example:
        >>> instance = get_instance("http://localhost:8080", 1)
        >>> print(f"Status: {instance['status']}")
    """
    return _api_get(base_url, f"/api/spammer/{instance_id}")


def start_instance(base_url: str, instance_id: int) -> dict[str, Any]:
    """Start a spammer instance.

    Args:
        base_url: Spamoor daemon base URL
        instance_id: Spammer instance ID to start

    Returns:
        Success response dictionary.

    Example:
        >>> result = start_instance("http://localhost:8080", 1)
        >>> print(f"Started: {result['success']}")
    """
    return _api_post(base_url, f"/api/spammer/{instance_id}/start")


def stop_instance(base_url: str, instance_id: int) -> dict[str, Any]:
    """Stop (pause) a spammer instance.

    Args:
        base_url: Spamoor daemon base URL
        instance_id: Spammer instance ID to stop

    Returns:
        Success response dictionary.

    Example:
        >>> result = stop_instance("http://localhost:8080", 1)
        >>> print(f"Stopped: {result['success']}")
    """
    return _api_post(base_url, f"/api/spammer/{instance_id}/pause")


def get_instance_logs(base_url: str, instance_id: int) -> list[dict[str, Any]]:
    """Get logs for a spammer instance.

    Args:
        base_url: Spamoor daemon base URL
        instance_id: Spammer instance ID

    Returns:
        List of log entries with time, level, message, and fields.

    Example:
        >>> logs = get_instance_logs("http://localhost:8080", 1)
        >>> for log in logs[-10:]:
        ...     print(f"[{log['level']}] {log['message']}")
    """
    return _api_get(base_url, f"/api/spammer/{instance_id}/logs")


# RPC Client management


def get_clients(base_url: str) -> list[dict[str, Any]]:
    """List all RPC clients.

    Args:
        base_url: Spamoor daemon base URL

    Returns:
        List of client dictionaries with index, name, groups, type,
        version, block_height, ready, rpc_host, and enabled fields.

    Example:
        >>> clients = get_clients("http://localhost:8080")
        >>> for client in clients:
        ...     status = "ready" if client['ready'] else "not ready"
        ...     print(f"{client['name']}: {status}")
    """
    return _api_get(base_url, "/api/clients")


# Wallet management


def get_wallets(base_url: str) -> list[dict[str, Any]]:
    """Get wallet information.

    Args:
        base_url: Spamoor daemon base URL

    Returns:
        List of wallet dictionaries with address, name, balance,
        nonce, and tx_count fields.

    Example:
        >>> wallets = get_wallets("http://localhost:8080")
        >>> for wallet in wallets:
        ...     print(f"{wallet['name']}: {wallet['balance']} wei")
    """
    return _api_get(base_url, "/api/wallets")


def get_pending_txs(base_url: str, wallet: str | None = None) -> list[dict[str, Any]]:
    """Get pending transactions.

    Args:
        base_url: Spamoor daemon base URL
        wallet: Optional wallet address to filter by

    Returns:
        List of pending transaction dictionaries with hash, wallet_address,
        wallet_name, nonce, value, fee, and submitted_at fields.

    Example:
        >>> pending = get_pending_txs("http://localhost:8080")
        >>> print(f"Pending transactions: {len(pending)}")
        >>> for tx in pending[:5]:
        ...     print(f"  {tx['hash'][:20]}... - {tx['value_formatted']}")
    """
    params = {}
    if wallet:
        params["wallet"] = wallet
    return _api_get(base_url, "/api/pending-transactions", params)


# Metrics


def get_metrics(base_url: str) -> dict[str, Any]:
    """Get spammer metrics dashboard.

    Args:
        base_url: Spamoor daemon base URL

    Returns:
        Metrics dictionary with range, spammers list, totals, others,
        and data_points for time-series data.

    Example:
        >>> metrics = get_metrics("http://localhost:8080")
        >>> print(f"Total confirmed: {metrics['totals']['confirmed']}")
        >>> for s in metrics['spammers']:
        ...     print(f"{s['name']}: {s['confirmed']} confirmed")
    """
    return _api_get(base_url, "/api/graphs/dashboard")


def get_scenarios(base_url: str) -> list[dict[str, Any]]:
    """Get available transaction scenarios.

    Args:
        base_url: Spamoor daemon base URL

    Returns:
        List of scenario dictionaries with name and description fields.

    Example:
        >>> scenarios = get_scenarios("http://localhost:8080")
        >>> for s in scenarios:
        ...     print(f"{s['name']}: {s['description']}")
    """
    return _api_get(base_url, "/api/scenarios")


# Deep links


def link_dashboard(base_url: str) -> str:
    """Generate deep link to Spamoor dashboard.

    Args:
        base_url: Spamoor daemon base URL

    Returns:
        URL to the dashboard page.

    Example:
        >>> link = link_dashboard("http://localhost:8080")
        >>> print(f"Dashboard: {link}")
    """
    return f"{base_url.rstrip('/')}/"


def link_instance(base_url: str, instance_id: int) -> str:
    """Generate deep link to a specific spammer instance.

    Args:
        base_url: Spamoor daemon base URL
        instance_id: Spammer instance ID

    Returns:
        URL to the spammer instance page.

    Example:
        >>> link = link_instance("http://localhost:8080", 1)
        >>> print(f"Spammer: {link}")
    """
    return f"{base_url.rstrip('/')}/?spammer={instance_id}"


def link_wallets(base_url: str) -> str:
    """Generate deep link to wallets page.

    Args:
        base_url: Spamoor daemon base URL

    Returns:
        URL to the wallets page.

    Example:
        >>> link = link_wallets("http://localhost:8080")
        >>> print(f"Wallets: {link}")
    """
    return f"{base_url.rstrip('/')}/wallets"


def link_clients(base_url: str) -> str:
    """Generate deep link to clients page.

    Args:
        base_url: Spamoor daemon base URL

    Returns:
        URL to the clients page.

    Example:
        >>> link = link_clients("http://localhost:8080")
        >>> print(f"Clients: {link}")
    """
    return f"{base_url.rstrip('/')}/clients"


def link_graphs(base_url: str) -> str:
    """Generate deep link to graphs/metrics page.

    Args:
        base_url: Spamoor daemon base URL

    Returns:
        URL to the graphs page.

    Example:
        >>> link = link_graphs("http://localhost:8080")
        >>> print(f"Graphs: {link}")
    """
    return f"{base_url.rstrip('/')}/graphs"
