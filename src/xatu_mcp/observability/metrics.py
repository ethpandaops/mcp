"""Prometheus metrics for the MCP server."""

from prometheus_client import Counter, Gauge, Histogram, REGISTRY, generate_latest
import structlog

logger = structlog.get_logger()


class Metrics:
    """Prometheus metrics collection for the MCP server."""

    def __init__(self) -> None:
        """Initialize metrics."""
        # Tool call metrics
        self.tool_calls_total = Counter(
            "mcp_tool_calls_total",
            "Total number of tool calls",
            ["tool", "status"],
        )

        self.tool_duration_seconds = Histogram(
            "mcp_tool_duration_seconds",
            "Tool execution duration in seconds",
            ["tool"],
            buckets=(0.1, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0, 120.0, 300.0),
        )

        # Sandbox metrics
        self.sandbox_duration_seconds = Histogram(
            "mcp_sandbox_duration_seconds",
            "Sandbox execution duration in seconds",
            ["backend"],
            buckets=(0.1, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0, 120.0, 300.0),
        )

        self.sandbox_containers_running = Gauge(
            "mcp_sandbox_containers_running",
            "Number of sandbox containers currently running",
        )

        # Query metrics (from sandbox)
        self.sandbox_queries_total = Counter(
            "mcp_sandbox_queries_total",
            "Total number of queries from sandbox",
            ["cluster", "network"],
        )

        self.sandbox_query_duration_seconds = Histogram(
            "mcp_sandbox_query_duration_seconds",
            "Query duration in sandbox",
            ["cluster", "network"],
            buckets=(0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0),
        )

        # Auth metrics
        self.auth_attempts_total = Counter(
            "mcp_auth_attempts_total",
            "Total authentication attempts",
            ["result"],
        )

        # Session metrics
        self.active_sessions = Gauge(
            "mcp_active_sessions",
            "Number of active sessions",
        )

        # Storage metrics
        self.storage_uploads_total = Counter(
            "mcp_storage_uploads_total",
            "Total number of files uploaded to storage",
            ["content_type"],
        )

        self.storage_upload_bytes_total = Counter(
            "mcp_storage_upload_bytes_total",
            "Total bytes uploaded to storage",
        )

    def record_tool_call(
        self,
        tool: str,
        status: str,
        duration_seconds: float,
    ) -> None:
        """Record a tool call.

        Args:
            tool: Tool name.
            status: Result status (success, error, timeout).
            duration_seconds: Execution duration.
        """
        self.tool_calls_total.labels(tool=tool, status=status).inc()
        self.tool_duration_seconds.labels(tool=tool).observe(duration_seconds)

    def record_sandbox_execution(
        self,
        backend: str,
        duration_seconds: float,
    ) -> None:
        """Record a sandbox execution.

        Args:
            backend: Sandbox backend name.
            duration_seconds: Execution duration.
        """
        self.sandbox_duration_seconds.labels(backend=backend).observe(duration_seconds)

    def record_sandbox_metrics(self, metrics: dict) -> None:
        """Record metrics collected from sandbox execution.

        Args:
            metrics: Metrics dictionary from sandbox .metrics.json.
        """
        # Parse and record query metrics
        for query_metric in metrics.get("queries", []):
            cluster = query_metric.get("cluster", "unknown")
            network = query_metric.get("network", "unknown")
            duration = query_metric.get("duration_seconds", 0)

            self.sandbox_queries_total.labels(cluster=cluster, network=network).inc()
            self.sandbox_query_duration_seconds.labels(
                cluster=cluster,
                network=network,
            ).observe(duration)

    def record_storage_upload(
        self,
        content_type: str,
        size_bytes: int,
    ) -> None:
        """Record a file upload.

        Args:
            content_type: MIME type of the uploaded file.
            size_bytes: Size of the file in bytes.
        """
        self.storage_uploads_total.labels(content_type=content_type).inc()
        self.storage_upload_bytes_total.inc(size_bytes)

    def record_auth_attempt(self, result: str) -> None:
        """Record an authentication attempt.

        Args:
            result: Result of the attempt (success, failure, invalid_org).
        """
        self.auth_attempts_total.labels(result=result).inc()

    def set_active_sessions(self, count: int) -> None:
        """Set the number of active sessions.

        Args:
            count: Number of active sessions.
        """
        self.active_sessions.set(count)

    def set_running_containers(self, count: int) -> None:
        """Set the number of running sandbox containers.

        Args:
            count: Number of running containers.
        """
        self.sandbox_containers_running.set(count)

    def generate_metrics(self) -> bytes:
        """Generate Prometheus metrics output.

        Returns:
            Prometheus text format metrics.
        """
        return generate_latest(REGISTRY)


# Global metrics instance
_metrics: Metrics | None = None


def get_metrics() -> Metrics:
    """Get or create the global metrics instance.

    Returns:
        Metrics instance.
    """
    global _metrics
    if _metrics is None:
        _metrics = Metrics()
    return _metrics
