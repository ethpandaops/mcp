"""CBT (ClickHouse Build Tool) access via credential proxy.

This module provides functions to query CBT for model metadata and
transformation state. All requests go through the credential proxy -
credentials are never exposed to sandbox containers.

Example:
    from ethpandaops import cbt

    # List available CBT instances
    instances = cbt.list_datasources()

    # List all models
    models = cbt.list_models()

    # Get model details
    model = cbt.get_model("analytics", "block_propagation")

    # Check transformation status
    status = cbt.get_transformation_status("analytics", "block_propagation")

    # Get UI link
    url = cbt.get_model_ui_url("analytics", "block_propagation")
"""

import json
import os
from typing import Any

import httpx

# Proxy configuration (required).
_PROXY_URL = os.environ.get("ETHPANDAOPS_PROXY_URL", "")
_PROXY_TOKEN = os.environ.get("ETHPANDAOPS_PROXY_TOKEN", "")

# Cache for instance info.
_INSTANCE_INFO: list[dict[str, str]] | None = None


def _check_proxy_config() -> None:
    """Verify proxy is configured."""
    if not _PROXY_URL or not _PROXY_TOKEN:
        raise ValueError(
            "Proxy not configured. ETHPANDAOPS_PROXY_URL and ETHPANDAOPS_PROXY_TOKEN are required."
        )


def _load_instances() -> list[dict[str, str]]:
    """Load instance info from environment variable."""
    global _INSTANCE_INFO

    if _INSTANCE_INFO is not None:
        return _INSTANCE_INFO

    raw = os.environ.get("ETHPANDAOPS_CBT_INSTANCES", "")
    if not raw:
        _INSTANCE_INFO = []
        return _INSTANCE_INFO

    try:
        _INSTANCE_INFO = json.loads(raw)
    except json.JSONDecodeError as e:
        raise ValueError(f"Invalid ETHPANDAOPS_CBT_INSTANCES JSON: {e}") from e

    return _INSTANCE_INFO


def _get_instance_names() -> list[str]:
    """Get list of instance names for validation."""
    return [inst["name"] for inst in _load_instances()]


def _get_default_instance() -> str | None:
    """Get the default instance name (first available)."""
    instances = _load_instances()
    if instances:
        return instances[0]["name"]
    return None


def _get_instance_url(instance_name: str) -> str:
    """Get the base URL for a CBT instance."""
    for inst in _load_instances():
        if inst["name"] == instance_name:
            return inst.get("url", "")
    raise ValueError(f"Unknown instance: {instance_name}")


def _get_instance_ui_url(instance_name: str) -> str:
    """Get the UI base URL for a CBT instance."""
    for inst in _load_instances():
        if inst["name"] == instance_name:
            return inst.get("ui_url", inst.get("url", ""))
    raise ValueError(f"Unknown instance: {instance_name}")


def _get_client(instance: str | None = None) -> httpx.Client:
    """Get an HTTP client configured for the proxy with the specified instance."""
    _check_proxy_config()

    # Determine instance to use.
    if instance is None:
        instance = _get_default_instance()
        if instance is None:
            raise ValueError("No CBT instances configured")

    # Validate instance.
    if instance not in _get_instance_names():
        raise ValueError(
            f"Unknown instance '{instance}'. Available instances: {_get_instance_names()}"
        )

    return httpx.Client(
        base_url=_PROXY_URL,
        headers={
            "Authorization": f"Bearer {_PROXY_TOKEN}",
            "X-Datasource": instance,
        },
        timeout=httpx.Timeout(connect=5.0, read=60.0, write=30.0, pool=5.0),
    )


def list_datasources() -> list[dict[str, Any]]:
    """List available CBT instances.

    Returns:
        List of instance info dictionaries with name, description, url, ui_url.

    Example:
        >>> instances = list_datasources()
        >>> for inst in instances:
        ...     print(f"{inst['name']}: {inst['description']}")
    """
    return _load_instances()


def list_models(
    instance: str | None = None,
    model_type: str | None = None,
    database: str | None = None,
) -> list[dict[str, Any]]:
    """List all models from CBT.

    Args:
        instance: CBT instance name (default: first available).
        model_type: Filter by type ('external', 'transformation', or None for all).
        database: Filter by database name.

    Returns:
        List of model dicts with database, table, type, dependencies, etc.

    Example:
        >>> models = cbt.list_models()
        >>> models = cbt.list_models(model_type="transformation", database="analytics")
    """
    params: dict[str, str] = {}
    if model_type:
        params["type"] = model_type
    if database:
        params["database"] = database

    with _get_client(instance) as client:
        response = client.get("/cbt/api/v1/models", params=params)
        response.raise_for_status()
        return response.json()


def get_model(database: str, table: str, instance: str | None = None) -> dict[str, Any]:
    """Get detailed information about a specific model.

    Args:
        database: Database name containing the model.
        table: Table name of the model.
        instance: CBT instance name (default: first available).

    Returns:
        Dict with model details including dependencies, interval config, schedules.

    Example:
        >>> model = cbt.get_model("analytics", "block_propagation")
        >>> print(f"Type: {model['type']}")
        >>> print(f"Dependencies: {model.get('dependencies', [])}")
    """
    model_id = f"{database}.{table}"

    with _get_client(instance) as client:
        response = client.get(f"/cbt/api/v1/models/{model_id}")
        response.raise_for_status()
        return response.json()


def get_transformation_status(
    database: str, table: str, instance: str | None = None
) -> dict[str, Any]:
    """Query the processing status of a transformation model.

    Args:
        database: Database name containing the model.
        table: Table name of the model.
        instance: CBT instance name (default: first available).

    Returns:
        Dict with 'total_intervals', 'min_position', 'max_position', 'total_gap_size',
        'coverage_percentage', and 'status'.

    Example:
        >>> status = cbt.get_transformation_status("analytics", "block_propagation")
        >>> print(f"Processed up to: {status.get('max_position', 'N/A')}")
        >>> print(f"Coverage: {status.get('coverage_percentage', 0):.1f}%")
    """
    model_id = f"{database}.{table}"

    with _get_client(instance) as client:
        response = client.get(f"/cbt/api/v1/models/{model_id}/status")
        response.raise_for_status()
        return response.json()


def get_model_ui_url(database: str, table: str, instance: str | None = None) -> str:
    """Get deep link to CBT UI for a specific model.

    Args:
        database: Database name containing the model.
        table: Table name of the model.
        instance: CBT instance name (default: first available).

    Returns:
        URL string to open the model in CBT UI.

    Example:
        >>> url = cbt.get_model_ui_url("analytics", "block_propagation")
        >>> print(f"Open UI: {url}")
    """
    # Determine instance to use.
    if instance is None:
        instance = _get_default_instance()
        if instance is None:
            raise ValueError("No CBT instances configured")

    base_url = _get_instance_ui_url(instance)
    return f"{base_url}/models/{database}/{table}"
