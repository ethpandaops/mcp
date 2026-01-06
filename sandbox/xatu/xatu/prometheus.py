"""Prometheus metrics access.

This module provides functions to query Prometheus for infrastructure
and node metrics.

Example:
    from xatu import prometheus

    # Instant query
    result = prometheus.query("up")

    # Range query
    result = prometheus.query_range(
        "rate(http_requests_total[5m])",
        start="now-1h",
        end="now",
        step="1m"
    )
"""

import os
from typing import Any

import httpx

_PROMETHEUS_URL = os.environ.get("XATU_PROMETHEUS_URL", "")


def _get_client() -> httpx.Client:
    """Get HTTP client for Prometheus."""
    if not _PROMETHEUS_URL:
        raise ValueError(
            "Prometheus not configured. Set XATU_PROMETHEUS_URL environment variable."
        )
    return httpx.Client(base_url=_PROMETHEUS_URL, timeout=30.0)


def query(promql: str, time: str | None = None) -> dict[str, Any]:
    """Execute an instant PromQL query.

    Args:
        promql: PromQL expression to evaluate.
        time: Evaluation timestamp (RFC3339 or Unix timestamp). Default: now.

    Returns:
        Query result as a dictionary with 'resultType' and 'result' keys.

    Example:
        >>> result = query("up")
        >>> result = query("rate(http_requests_total[5m])", time="2024-01-01T00:00:00Z")
    """
    with _get_client() as client:
        params: dict[str, str] = {"query": promql}
        if time:
            params["time"] = time

        response = client.get("/api/v1/query", params=params)
        response.raise_for_status()

        data = response.json()

        if data.get("status") != "success":
            raise ValueError(f"Prometheus query failed: {data.get('error', 'Unknown error')}")

        return data["data"]


def query_range(
    promql: str,
    start: str,
    end: str,
    step: str,
) -> dict[str, Any]:
    """Execute a range PromQL query.

    Args:
        promql: PromQL expression to evaluate.
        start: Start timestamp (RFC3339 or Unix timestamp).
        end: End timestamp (RFC3339 or Unix timestamp).
        step: Query resolution step (e.g., "1m", "5m", "1h").

    Returns:
        Query result as a dictionary with 'resultType' and 'result' keys.

    Example:
        >>> result = query_range(
        ...     "rate(http_requests_total[5m])",
        ...     start="now-1h",
        ...     end="now",
        ...     step="1m"
        ... )
    """
    with _get_client() as client:
        params = {
            "query": promql,
            "start": start,
            "end": end,
            "step": step,
        }

        response = client.get("/api/v1/query_range", params=params)
        response.raise_for_status()

        data = response.json()

        if data.get("status") != "success":
            raise ValueError(f"Prometheus query failed: {data.get('error', 'Unknown error')}")

        return data["data"]


def get_labels() -> list[str]:
    """Get all label names.

    Returns:
        List of label names.
    """
    with _get_client() as client:
        response = client.get("/api/v1/labels")
        response.raise_for_status()

        data = response.json()

        if data.get("status") != "success":
            raise ValueError(f"Failed to get labels: {data.get('error', 'Unknown error')}")

        return data["data"]


def get_label_values(label: str) -> list[str]:
    """Get all values for a label.

    Args:
        label: Label name.

    Returns:
        List of label values.
    """
    with _get_client() as client:
        response = client.get(f"/api/v1/label/{label}/values")
        response.raise_for_status()

        data = response.json()

        if data.get("status") != "success":
            raise ValueError(f"Failed to get label values: {data.get('error', 'Unknown error')}")

        return data["data"]
