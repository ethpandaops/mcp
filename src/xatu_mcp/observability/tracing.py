"""OpenTelemetry tracing for the MCP server."""

from contextlib import contextmanager
from typing import Any, Generator

import structlog

logger = structlog.get_logger()

# Try to import OpenTelemetry, but make it optional
try:
    from opentelemetry import trace
    from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
    from opentelemetry.sdk.resources import Resource
    from opentelemetry.sdk.trace import TracerProvider
    from opentelemetry.sdk.trace.export import BatchSpanProcessor

    OTEL_AVAILABLE = True
except ImportError:
    OTEL_AVAILABLE = False


class Tracer:
    """OpenTelemetry tracer for the MCP server."""

    def __init__(
        self,
        service_name: str = "xatu-mcp",
        otlp_endpoint: str | None = None,
    ) -> None:
        """Initialize the tracer.

        Args:
            service_name: Service name for traces.
            otlp_endpoint: OTLP exporter endpoint. If None, tracing is disabled.
        """
        self.service_name = service_name
        self.otlp_endpoint = otlp_endpoint
        self._tracer: Any = None
        self._enabled = False

        if otlp_endpoint and OTEL_AVAILABLE:
            self._setup_tracing()
        elif otlp_endpoint and not OTEL_AVAILABLE:
            logger.warning(
                "OpenTelemetry not available, tracing disabled. "
                "Install opentelemetry-* packages to enable."
            )

    def _setup_tracing(self) -> None:
        """Set up OpenTelemetry tracing."""
        if not OTEL_AVAILABLE:
            return

        resource = Resource.create({"service.name": self.service_name})

        provider = TracerProvider(resource=resource)

        exporter = OTLPSpanExporter(endpoint=self.otlp_endpoint)
        processor = BatchSpanProcessor(exporter)
        provider.add_span_processor(processor)

        trace.set_tracer_provider(provider)
        self._tracer = trace.get_tracer(self.service_name)
        self._enabled = True

        logger.info(
            "OpenTelemetry tracing enabled",
            endpoint=self.otlp_endpoint,
            service=self.service_name,
        )

    @property
    def enabled(self) -> bool:
        """Check if tracing is enabled."""
        return self._enabled

    @contextmanager
    def span(
        self,
        name: str,
        attributes: dict[str, Any] | None = None,
    ) -> Generator[Any, None, None]:
        """Create a trace span.

        Args:
            name: Span name.
            attributes: Span attributes.

        Yields:
            The span (or None if tracing is disabled).
        """
        if not self._enabled or not self._tracer:
            yield None
            return

        with self._tracer.start_as_current_span(name) as span:
            if attributes:
                for key, value in attributes.items():
                    span.set_attribute(key, value)
            yield span

    def add_event(
        self,
        name: str,
        attributes: dict[str, Any] | None = None,
    ) -> None:
        """Add an event to the current span.

        Args:
            name: Event name.
            attributes: Event attributes.
        """
        if not self._enabled:
            return

        span = trace.get_current_span()
        if span:
            span.add_event(name, attributes or {})

    def set_attribute(self, key: str, value: Any) -> None:
        """Set an attribute on the current span.

        Args:
            key: Attribute key.
            value: Attribute value.
        """
        if not self._enabled:
            return

        span = trace.get_current_span()
        if span:
            span.set_attribute(key, value)

    def get_trace_context(self) -> dict[str, str]:
        """Get the current trace context for propagation.

        Returns:
            Dictionary with traceparent and tracestate headers.
        """
        if not self._enabled:
            return {}

        from opentelemetry import propagate

        carrier: dict[str, str] = {}
        propagate.inject(carrier)
        return carrier


# Global tracer instance
_tracer: Tracer | None = None


def get_tracer(
    service_name: str = "xatu-mcp",
    otlp_endpoint: str | None = None,
) -> Tracer:
    """Get or create the global tracer instance.

    Args:
        service_name: Service name for traces.
        otlp_endpoint: OTLP exporter endpoint.

    Returns:
        Tracer instance.
    """
    global _tracer
    if _tracer is None:
        _tracer = Tracer(service_name, otlp_endpoint)
    return _tracer
