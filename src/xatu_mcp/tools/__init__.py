"""MCP tools for the Xatu server.

This module provides a unified tool registration system that correctly handles
multiple tools without the handler overwriting issues that occur when using
separate @server.list_tools() and @server.call_tool() decorators.
"""

from typing import Any

from mcp.server import Server
from mcp.types import TextContent, Tool
import structlog

from xatu_mcp.config import Config
from xatu_mcp.sandbox.base import SandboxBackend

logger = structlog.get_logger()


def register_all_tools(server: Server, sandbox: SandboxBackend, config: Config) -> None:
    """Register all MCP tools with the server.

    This function registers all tools in a single handler to avoid the issue where
    multiple @server.list_tools() decorators overwrite each other.

    Args:
        server: The MCP server instance.
        sandbox: The sandbox backend for code execution.
        config: Server configuration.
    """
    from xatu_mcp.tools.execute_python import build_execute_python_tool, handle_execute_python
    from xatu_mcp.tools.files import build_file_tools, handle_get_output_file, handle_list_output_files

    @server.list_tools()
    async def list_tools() -> list[Tool]:
        """Return all available tools."""
        tools = []

        # Add execute_python tool
        tools.append(build_execute_python_tool())

        # Add file tools
        tools.extend(build_file_tools())

        return tools

    @server.call_tool()
    async def call_tool(name: str, arguments: dict[str, Any]) -> list[TextContent]:
        """Route tool calls to appropriate handlers."""
        if name == "execute_python":
            return await handle_execute_python(arguments, sandbox, config)
        elif name == "get_output_file":
            return await handle_get_output_file(arguments, config)
        elif name == "list_output_files":
            return await handle_list_output_files(arguments, config)
        else:
            raise ValueError(f"Unknown tool: {name}")


__all__ = ["register_all_tools"]
