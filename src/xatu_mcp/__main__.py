"""CLI entry point for the Xatu MCP server."""

import argparse
import asyncio
import sys
from pathlib import Path

import structlog

from xatu_mcp.config import load_config
from xatu_mcp.server import create_server, run_stdio, run_sse, run_streamable_http

logger = structlog.get_logger()


def main() -> int:
    """Main entry point for the CLI."""
    parser = argparse.ArgumentParser(
        prog="xatu-mcp",
        description="MCP server for Ethereum network analytics via Xatu data",
    )

    parser.add_argument(
        "--config",
        "-c",
        type=Path,
        help="Path to config file (default: CONFIG_PATH env var or config.yaml)",
    )

    subparsers = parser.add_subparsers(dest="command", help="Available commands")

    # serve command
    serve_parser = subparsers.add_parser("serve", help="Start the MCP server")
    serve_parser.add_argument(
        "--transport",
        "-t",
        choices=["stdio", "sse", "streamable-http"],
        default="stdio",
        help="Transport protocol (default: stdio)",
    )
    serve_parser.add_argument(
        "--host",
        default=None,
        help="Host to bind to (overrides config)",
    )
    serve_parser.add_argument(
        "--port",
        type=int,
        default=None,
        help="Port to bind to (overrides config)",
    )

    # schema command
    schema_parser = subparsers.add_parser("schema", help="Manage ClickHouse schemas")
    schema_subparsers = schema_parser.add_subparsers(dest="schema_command")
    refresh_parser = schema_subparsers.add_parser("refresh", help="Refresh schemas from ClickHouse")
    refresh_parser.add_argument(
        "--cluster",
        choices=["xatu", "xatu-experimental", "xatu-cbt"],
        help="Specific cluster to refresh (default: all)",
    )

    # version command
    subparsers.add_parser("version", help="Show version")

    args = parser.parse_args()

    # Configure structured logging
    structlog.configure(
        processors=[
            structlog.stdlib.filter_by_level,
            structlog.stdlib.add_logger_name,
            structlog.stdlib.add_log_level,
            structlog.processors.TimeStamper(fmt="iso"),
            structlog.processors.StackInfoRenderer(),
            structlog.processors.format_exc_info,
            structlog.dev.ConsoleRenderer() if sys.stderr.isatty() else structlog.processors.JSONRenderer(),
        ],
        wrapper_class=structlog.stdlib.BoundLogger,
        context_class=dict,
        logger_factory=structlog.stdlib.LoggerFactory(),
        cache_logger_on_first_use=True,
    )

    if args.command == "version":
        from xatu_mcp import __version__

        print(f"xatu-mcp {__version__}")
        return 0

    if args.command == "schema":
        return handle_schema_command(args)

    if args.command == "serve" or args.command is None:
        return handle_serve_command(args)

    parser.print_help()
    return 1


def handle_serve_command(args: argparse.Namespace) -> int:
    """Handle the serve command."""
    try:
        config = load_config(args.config)
    except FileNotFoundError as e:
        logger.error("Config file not found", error=str(e))
        return 1
    except ValueError as e:
        logger.error("Config error", error=str(e))
        return 1

    # Apply CLI overrides
    if hasattr(args, "host") and args.host:
        config.server.host = args.host
    if hasattr(args, "port") and args.port:
        config.server.port = args.port

    transport = getattr(args, "transport", "stdio")

    logger.info(
        "Starting Xatu MCP server",
        transport=transport,
        host=config.server.host,
        port=config.server.port,
    )

    server = create_server(config)

    try:
        if transport == "stdio":
            asyncio.run(run_stdio(server))
        elif transport == "sse":
            asyncio.run(run_sse(server, config))
        elif transport == "streamable-http":
            asyncio.run(run_streamable_http(server, config))
        else:
            logger.error("Unknown transport", transport=transport)
            return 1
    except KeyboardInterrupt:
        logger.info("Server stopped")
    except Exception as e:
        logger.exception("Server error", error=str(e))
        return 1

    return 0


def handle_schema_command(args: argparse.Namespace) -> int:
    """Handle the schema command."""
    if args.schema_command == "refresh":
        try:
            config = load_config(args.config)
        except FileNotFoundError as e:
            logger.error("Config file not found", error=str(e))
            return 1
        except ValueError as e:
            logger.error("Config error", error=str(e))
            return 1

        cluster = getattr(args, "cluster", None)
        logger.info("Refreshing schemas", cluster=cluster or "all")

        # TODO: Implement schema refresh
        logger.warning("Schema refresh not yet implemented")
        return 0

    logger.error("Unknown schema command")
    return 1


if __name__ == "__main__":
    sys.exit(main())
