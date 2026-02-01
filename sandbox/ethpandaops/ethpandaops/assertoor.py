"""Assertoor testnet testing tool access.

Access the Assertoor testnet testing tool API and generate deep links.
Network URLs are discovered from cartographoor via environment variables.

Example:
    from ethpandaops import assertoor

    networks = assertoor.list_networks()
    test_runs = assertoor.get_test_runs("holesky")
    link = assertoor.link_test_run("holesky", 123)
"""

import json
import os
from typing import Any

import httpx

_NETWORKS: dict[str, str] | None = None
_TIMEOUT = httpx.Timeout(connect=5.0, read=30.0, write=10.0, pool=5.0)


def _load_networks() -> dict[str, str]:
    """Load network -> Assertoor URL mapping from environment."""
    global _NETWORKS
    if _NETWORKS is not None:
        return _NETWORKS

    raw = os.environ.get("ETHPANDAOPS_ASSERTOOR_NETWORKS", "")
    if not raw:
        _NETWORKS = {}
        return _NETWORKS

    try:
        _NETWORKS = json.loads(raw)
    except json.JSONDecodeError as e:
        raise ValueError(f"Invalid ETHPANDAOPS_ASSERTOOR_NETWORKS JSON: {e}") from e

    return _NETWORKS


def _get_url(network: str) -> str:
    """Get Assertoor base URL for a network."""
    networks = _load_networks()
    if network not in networks:
        raise ValueError(f"Unknown network '{network}'. Available: {list(networks.keys())}")
    return networks[network]


def _api_get(network: str, path: str, params: dict[str, Any] | None = None) -> dict[str, Any]:
    """Make GET request to Assertoor API."""
    url = f"{_get_url(network)}{path}"
    with httpx.Client(timeout=_TIMEOUT) as client:
        resp = client.get(url, params=params)
        resp.raise_for_status()
        return resp.json()


# Network discovery


def list_networks() -> list[dict[str, str]]:
    """List networks with Assertoor instances."""
    return [{"name": n, "assertoor_url": u} for n, u in sorted(_load_networks().items())]


def get_base_url(network: str) -> str:
    """Get Assertoor base URL for a network."""
    return _get_url(network)


# API queries


def get_tests(network: str) -> list[dict[str, Any]]:
    """List available test definitions.

    Returns a list of test definitions that can be used to create test runs.
    Each test definition includes:
        - id: Test ID
        - source: Source of the test (e.g., "config", "external")
        - basePath: Base path for the test
        - name: Human-readable test name
    """
    result = _api_get(network, "/api/v1/tests")
    data = result.get("data", [])
    return data if isinstance(data, list) else []


def get_test_runs(network: str, test_id: str | None = None, limit: int = 100) -> list[dict[str, Any]]:
    """List test runs with optional filter.

    Args:
        network: Network name (e.g., "holesky")
        test_id: Optional test ID to filter by
        limit: Maximum number of results to return

    Returns a list of test runs, each containing:
        - run_id: Unique run ID
        - test_id: Test definition ID
        - name: Test name
        - status: Test status (pending, running, success, failure, skipped, aborted)
        - start_time: Start time as Unix timestamp
        - stop_time: Stop time as Unix timestamp (0 if not completed)
    """
    params: dict[str, Any] = {"limit": limit}
    if test_id:
        params["test_id"] = test_id

    result = _api_get(network, "/api/v1/test_runs", params)
    data = result.get("data", [])
    return data if isinstance(data, list) else []


def get_test_run_details(network: str, run_id: int) -> dict[str, Any]:
    """Get detailed test run information.

    Args:
        network: Network name (e.g., "holesky")
        run_id: Test run ID

    Returns detailed test run information including:
        - run_id: Unique run ID
        - test_id: Test definition ID
        - name: Test name
        - status: Test status
        - start_time: Start time as Unix timestamp
        - stop_time: Stop time as Unix timestamp
        - tasks: List of tasks with their status and logs
    """
    result = _api_get(network, f"/api/v1/test_run/{run_id}/details")
    return result.get("data", {})


def get_test_run_status(network: str, run_id: int) -> dict[str, Any]:
    """Get test run status summary.

    Args:
        network: Network name (e.g., "holesky")
        run_id: Test run ID

    Returns test run status containing:
        - run_id: Unique run ID
        - test_id: Test definition ID
        - name: Test name
        - status: Test status
        - start_time: Start time as Unix timestamp
        - stop_time: Stop time as Unix timestamp
    """
    result = _api_get(network, f"/api/v1/test_run/{run_id}/status")
    return result.get("data", {})


def get_task_details(network: str, run_id: int, task_index: int) -> dict[str, Any]:
    """Get task details with logs.

    Args:
        network: Network name (e.g., "holesky")
        run_id: Test run ID
        task_index: Task index within the test run

    Returns detailed task information including:
        - index: Task index
        - parent_index: Parent task index
        - name: Task name
        - title: Task title
        - started: Whether task has started
        - completed: Whether task has completed
        - start_time: Start time as Unix timestamp
        - stop_time: Stop time as Unix timestamp
        - status: Task status (pending, running, complete)
        - result: Task result (none, success, failure)
        - result_error: Error message if task failed
        - log: List of log entries
        - config_yaml: Task configuration as YAML
        - result_yaml: Task result as YAML
    """
    result = _api_get(network, f"/api/v1/test_run/{run_id}/task/{task_index}/details")
    return result.get("data", {})


# Deep links


def link_test_run(network: str, run_id: int) -> str:
    """Generate link to test run page.

    Args:
        network: Network name (e.g., "holesky")
        run_id: Test run ID

    Returns a deep link to the Assertoor web UI for the test run.
    """
    return f"{_get_url(network)}/test/{run_id}"


def link_task(network: str, run_id: int, task_index: int) -> str:
    """Generate link to task page.

    Args:
        network: Network name (e.g., "holesky")
        run_id: Test run ID
        task_index: Task index within the test run

    Returns a deep link to the Assertoor web UI for the task.
    """
    return f"{_get_url(network)}/test/{run_id}?task={task_index}"


def link_task_logs(network: str, run_id: int, task_index: int) -> str:
    """Generate link to task logs page.

    Args:
        network: Network name (e.g., "holesky")
        run_id: Test run ID
        task_index: Task index within the test run

    Returns a deep link to the Assertoor web UI for the task logs.
    """
    return f"{_get_url(network)}/test/{run_id}?task={task_index}&logs=1"
