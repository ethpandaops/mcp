"""MCP server setup and transport runners."""

from typing import Any

from mcp.server import Server
from mcp.server.stdio import stdio_server
import structlog

from xatu_mcp.config import Config
from xatu_mcp.sandbox import DockerBackend, GVisorBackend, SandboxBackend

logger = structlog.get_logger()


# Global authorization server instance (set when auth is enabled)
_auth_server: Any = None


def create_auth_server(config: Config) -> Any:
    """Create the authorization server if auth is enabled.

    Args:
        config: Server configuration.

    Returns:
        AuthorizationServer instance if auth is enabled, None otherwise.
    """
    global _auth_server

    if not config.auth.enabled:
        logger.info("Authentication disabled")
        return None

    if not config.auth.github:
        raise ValueError("GitHub OAuth configuration is required when auth is enabled")

    from xatu_mcp.auth import AuthorizationServer

    _auth_server = AuthorizationServer(
        config=config.auth,
        base_url=config.server.base_url,
    )

    logger.info(
        "Authorization server created",
        base_url=config.server.base_url,
        allowed_orgs=config.auth.allowed_orgs,
    )

    return _auth_server


def get_auth_server() -> Any:
    """Get the global authorization server instance.

    Returns:
        AuthorizationServer instance or None if not initialized.
    """
    return _auth_server


def create_sandbox_backend(config: Config) -> SandboxBackend:
    """Create the appropriate sandbox backend based on config."""
    backend_type = config.sandbox.backend.lower()

    if backend_type == "docker":
        return DockerBackend(
            image=config.sandbox.image,
            timeout=config.sandbox.timeout,
            memory_limit=config.sandbox.memory_limit,
            cpu_limit=config.sandbox.cpu_limit,
            network=config.sandbox.network,
        )
    elif backend_type == "gvisor":
        return GVisorBackend(
            image=config.sandbox.image,
            timeout=config.sandbox.timeout,
            memory_limit=config.sandbox.memory_limit,
            cpu_limit=config.sandbox.cpu_limit,
            network=config.sandbox.network,
        )
    else:
        raise ValueError(f"Unknown sandbox backend: {backend_type}")


def create_server(config: Config) -> Server:
    """Create and configure the MCP server.

    Args:
        config: Server configuration.

    Returns:
        Configured MCP server instance.
    """
    server = Server("xatu-mcp")

    # Register ClickHouse clusters from config (must be done before resources)
    from xatu_mcp.resources.clickhouse_client import register_clusters_from_config

    register_clusters_from_config(config)

    # Create sandbox backend
    sandbox = create_sandbox_backend(config)

    # Register all tools in a unified handler (fixes handler overwriting issue)
    from xatu_mcp.tools import register_all_tools

    register_all_tools(server, sandbox, config)

    # Register resources
    from xatu_mcp.resources import register_resources

    register_resources(server)

    logger.info(
        "MCP server created",
        sandbox_backend=config.sandbox.backend,
        auth_enabled=config.auth.enabled,
    )

    return server


async def run_stdio(server: Server) -> None:
    """Run the server using stdio transport.

    Args:
        server: The MCP server instance.
    """
    logger.info("Starting stdio transport")

    async with stdio_server() as (read_stream, write_stream):
        await server.run(
            read_stream,
            write_stream,
            server.create_initialization_options(),
        )


async def run_sse(server: Server, config: Config) -> None:
    """Run the server using SSE transport.

    Args:
        server: The MCP server instance.
        config: Server configuration.
    """
    from mcp.server.sse import SseServerTransport
    from starlette.applications import Starlette
    from starlette.routing import Route
    from starlette.responses import JSONResponse
    import uvicorn

    logger.info("Starting SSE transport", host=config.server.host, port=config.server.port)

    sse = SseServerTransport("/messages/")

    async def handle_sse(request):
        async with sse.connect_sse(
            request.scope,
            request.receive,
            request._send,
        ) as streams:
            await server.run(
                streams[0],
                streams[1],
                server.create_initialization_options(),
            )

    async def health_check(request):
        return JSONResponse({"status": "healthy"})

    # Build routes
    routes = [
        Route("/sse", endpoint=handle_sse),
        Route("/messages/", endpoint=sse.handle_post_message, methods=["POST"]),
        Route("/health", endpoint=health_check),
    ]

    # Add auth routes if enabled
    auth_server = create_auth_server(config)
    if auth_server:
        routes.extend(auth_server.get_routes())

    app = Starlette(routes=routes)

    # Add auth middleware if enabled
    if auth_server:
        from xatu_mcp.auth import AuthenticationMiddleware

        app.add_middleware(
            AuthenticationMiddleware,
            config=config.auth,
            token_manager=auth_server.token_manager,
            store=auth_server.store,
            base_url=config.server.base_url,
        )

    uvicorn_config = uvicorn.Config(
        app,
        host=config.server.host,
        port=config.server.port,
        log_level="info",
    )
    server_instance = uvicorn.Server(uvicorn_config)
    await server_instance.serve()


async def run_streamable_http(server: Server, config: Config) -> None:
    """Run the server using Streamable HTTP transport.

    Args:
        server: The MCP server instance.
        config: Server configuration.
    """
    from mcp.server.streamable_http import StreamableHTTPServerTransport
    from starlette.applications import Starlette
    from starlette.routing import Route
    from starlette.responses import JSONResponse
    import uvicorn

    logger.info(
        "Starting Streamable HTTP transport",
        host=config.server.host,
        port=config.server.port,
    )

    transport = StreamableHTTPServerTransport(
        "/mcp",
        server.create_initialization_options(),
    )

    async def handle_mcp(request):
        return await transport.handle_request(
            request.scope,
            request.receive,
            request._send,
            lambda: server,
        )

    async def health_check(request):
        return JSONResponse({"status": "healthy"})

    async def ready_check(request):
        return JSONResponse({"status": "ready"})

    # Build routes
    routes = [
        Route("/mcp", endpoint=handle_mcp, methods=["GET", "POST"]),
        Route("/health", endpoint=health_check),
        Route("/ready", endpoint=ready_check),
    ]

    # Add auth routes if enabled
    auth_server = create_auth_server(config)
    if auth_server:
        routes.extend(auth_server.get_routes())

    app = Starlette(routes=routes)

    # Add auth middleware if enabled
    if auth_server:
        from xatu_mcp.auth import AuthenticationMiddleware

        app.add_middleware(
            AuthenticationMiddleware,
            config=config.auth,
            token_manager=auth_server.token_manager,
            store=auth_server.store,
            base_url=config.server.base_url,
        )

    uvicorn_config = uvicorn.Config(
        app,
        host=config.server.host,
        port=config.server.port,
        log_level="info",
    )
    server_instance = uvicorn.Server(uvicorn_config)
    await server_instance.serve()
