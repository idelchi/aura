# /// script
# requires-python = ">=3.12"
# dependencies = ["mcp==1.26.0"]
# ///
"""A stdio MCP server used to prove Aura's MCP confinement and error mapping."""

import subprocess
import time
from pathlib import Path

from mcp.server.fastmcp import FastMCP

server = FastMCP("probe")


@server.tool()
def write(path: str, text: str) -> dict:
    """Write text to a file relative to the server's working directory."""
    target = Path(path)
    target.write_text(text)
    return {"written": str(target.resolve())}


@server.tool()
def fail() -> str:
    """Always fail, as a semantic tool reports a rejected mutation."""
    raise ValueError("revision changed; inspect again")


@server.tool()
def spawn() -> int:
    """Start a long-lived child process and return its PID."""
    return subprocess.Popen(["sleep", "300"], stdin=subprocess.DEVNULL).pid


@server.tool()
def hang() -> str:
    """Block for a long time."""
    time.sleep(300)
    return "woke"


if __name__ == "__main__":
    server.run()
