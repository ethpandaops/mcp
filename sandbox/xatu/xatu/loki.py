"""Loki log access.

This module provides functions to query Loki for log data.

Example:
    from xatu import loki

    # Query logs
    logs = loki.query('{app="beacon-node"}', limit=100)

    # Query with time range
    logs = loki.query(
        '{app="beacon-node"} |= "error"',
        start="now-1h",
        end="now",
        limit=50
    )
"""

import os
from typing import Any

import httpx

_LOKI_URL = os.environ.get("XATU_LOKI_URL", "")


def _get_client() -> httpx.Client:
    """Get HTTP client for Loki."""
    if not _LOKI_URL:
        raise ValueError("Loki not configured. Set XATU_LOKI_URL environment variable.")
    return httpx.Client(base_url=_LOKI_URL, timeout=30.0)


def query(
    logql: str,
    limit: int = 100,
    start: str | None = None,
    end: str | None = None,
    direction: str = "backward",
) -> list[dict[str, Any]]:
    """Execute a LogQL query.

    Args:
        logql: LogQL query string.
        limit: Maximum number of log lines to return (default: 100).
        start: Start time (RFC3339 or "now-1h" format). Default: now-1h.
        end: End time (RFC3339 or "now" format). Default: now.
        direction: Sort direction: "forward" (oldest first) or "backward" (newest first).

    Returns:
        List of log entries, each with 'timestamp', 'labels', and 'line' keys.

    Example:
        >>> logs = query('{app="beacon-node"}', limit=10)
        >>> logs = query('{app="beacon-node"} |= "error"', start="now-1h", limit=50)
    """
    with _get_client() as client:
        params: dict[str, Any] = {
            "query": logql,
            "limit": limit,
            "direction": direction,
        }

        if start:
            params["start"] = start
        if end:
            params["end"] = end

        response = client.get("/loki/api/v1/query_range", params=params)
        response.raise_for_status()

        data = response.json()

        if data.get("status") != "success":
            raise ValueError(f"Loki query failed: {data.get('error', 'Unknown error')}")

        # Parse the results
        results = []
        for stream in data.get("data", {}).get("result", []):
            labels = stream.get("stream", {})
            for value in stream.get("values", []):
                timestamp, line = value
                results.append({
                    "timestamp": timestamp,
                    "labels": labels,
                    "line": line,
                })

        return results


def get_labels(start: str | None = None, end: str | None = None) -> list[str]:
    """Get all label names.

    Args:
        start: Start time for label discovery.
        end: End time for label discovery.

    Returns:
        List of label names.
    """
    with _get_client() as client:
        params: dict[str, str] = {}
        if start:
            params["start"] = start
        if end:
            params["end"] = end

        response = client.get("/loki/api/v1/labels", params=params)
        response.raise_for_status()

        data = response.json()

        if data.get("status") != "success":
            raise ValueError(f"Failed to get labels: {data.get('error', 'Unknown error')}")

        return data["data"]


def get_label_values(
    label: str,
    start: str | None = None,
    end: str | None = None,
) -> list[str]:
    """Get all values for a label.

    Args:
        label: Label name.
        start: Start time for value discovery.
        end: End time for value discovery.

    Returns:
        List of label values.
    """
    with _get_client() as client:
        params: dict[str, str] = {}
        if start:
            params["start"] = start
        if end:
            params["end"] = end

        response = client.get(f"/loki/api/v1/label/{label}/values", params=params)
        response.raise_for_status()

        data = response.json()

        if data.get("status") != "success":
            raise ValueError(f"Failed to get label values: {data.get('error', 'Unknown error')}")

        return data["data"]
