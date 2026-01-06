"""MCP server setup and transport runners."""

from mcp.server import Server
from mcp.server.stdio import stdio_server
import structlog

from xatu_mcp.config import Config
from xatu_mcp.sandbox import DockerBackend, GVisorBackend, SandboxBackend

logger = structlog.get_logger()


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

    # Create sandbox backend
    sandbox = create_sandbox_backend(config)

    # Register tools
    from xatu_mcp.tools.execute_python import register_execute_python
    from xatu_mcp.tools.files import register_file_tools

    register_execute_python(server, sandbox, config)
    register_file_tools(server, config)

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

    app = Starlette(
        routes=[
            Route("/sse", endpoint=handle_sse),
            Route("/messages/", endpoint=sse.handle_post_message, methods=["POST"]),
        ],
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
        from starlette.responses import JSONResponse

        return JSONResponse({"status": "healthy"})

    app = Starlette(
        routes=[
            Route("/mcp", endpoint=handle_mcp, methods=["GET", "POST"]),
            Route("/health", endpoint=health_check),
        ],
    )

    uvicorn_config = uvicorn.Config(
        app,
        host=config.server.host,
        port=config.server.port,
        log_level="info",
    )
    server_instance = uvicorn.Server(uvicorn_config)
    await server_instance.serve()
