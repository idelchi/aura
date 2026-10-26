"""The launcher lease: a server cannot outlive a launcher that never recorded it."""

import json
import os
import subprocess
import sys
import time
from pathlib import Path

from aura_automation import Server
from aura_automation.server import _pid_alive

from .conftest import TOKEN, Workspace

LAUNCHER = """
import json, os, sys, threading
from pathlib import Path
from aura_automation import Aura, Server

spec = json.load(sys.stdin)
aura = Aura(binary=Path(spec["binary"]), args=spec["args"], cwd=Path(spec["cwd"]), env=spec["env"])
if spec["die_after"] is not None:
    threading.Timer(spec["die_after"], lambda: os._exit(3)).start()
handle = Server.launch(aura, ready_file=Path(spec["ready"]), token=spec["token"], log=Path(spec["log"]), hold=True)
if spec["release"]:
    handle.release()
print(json.dumps({"pid": handle.pid, "url": handle.url}), flush=True)
os._exit(0)
"""


def run_launcher(ws: Workspace, *, release: bool = False, die_after: float | None = None) -> subprocess.CompletedProcess:
    aura = ws.aura
    spec = {
        "binary": str(aura.binary), "args": list(aura.args), "cwd": str(aura.cwd), "env": dict(aura.env),
        "ready": str(ws.root / "ready.json"), "log": str(ws.root / "serve.log"), "token": TOKEN,
        "release": release, "die_after": die_after,
    }
    return subprocess.run([sys.executable, "-c", LAUNCHER], input=json.dumps(spec), capture_output=True, text=True,
                          timeout=120, check=False)


def serve_processes(ready: Path) -> list[int]:
    """Live processes started with this ready file."""
    pids = []
    for proc in Path("/proc").iterdir():
        if not proc.name.isdigit():
            continue
        try:
            cmdline = (proc / "cmdline").read_bytes()
        except OSError:
            continue
        if str(ready).encode() in cmdline and _pid_alive(int(proc.name)):
            pids.append(int(proc.name))
    return pids


def wait_until(condition, timeout: float = 30) -> bool:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if condition():
            return True
        time.sleep(0.1)
    return condition()


def test_launcher_death_during_startup_stops_the_server(workspace: Workspace) -> None:
    # A required MCP server that never answers keeps startup busy until the launcher dies.
    mcp = workspace.config / "config" / "mcp"
    mcp.mkdir(parents=True)
    (mcp / "stuck.yaml").write_text(json.dumps({"stuck": {"command": "sleep", "args": ["300"], "required": True, "timeout": "120s"}}))
    ready = workspace.root / "ready.json"

    launcher = run_launcher(workspace, die_after=2)
    assert launcher.returncode == 3, launcher.stderr

    assert wait_until(lambda: ready.exists() and not serve_processes(ready)), "server outlived its launcher"
    assert json.loads(ready.read_text())["error"]["code"] == "launcher_lost"


def test_launcher_death_before_release_stops_the_server(workspace: Workspace) -> None:
    launcher = run_launcher(workspace)
    assert launcher.returncode == 0, launcher.stderr
    pid = json.loads(launcher.stdout)["pid"]
    session = json.loads((workspace.root / "ready.json").read_text())["session_id"]

    assert wait_until(lambda: not _pid_alive(pid)), "unreleased server survived its launcher"
    stopping = [json.loads(line) for line in workspace.journal_path(session).read_text().splitlines()]
    assert any(e["type"] == "server.stopping" and e["data"]["reason"] == "launcher_lost" for e in stopping)


def test_released_server_outlives_its_launcher(workspace: Workspace) -> None:
    workspace.write_script({"content": "still here"})
    launcher = run_launcher(workspace, release=True)
    assert launcher.returncode == 0, launcher.stderr
    pid = json.loads(launcher.stdout)["pid"]

    time.sleep(2)
    assert _pid_alive(pid), "released server died with its launcher"
    handle = Server.attach(workspace.root / "ready.json", TOKEN)
    workspace.servers.append(handle)
    with handle.client() as client:
        assert client.wait(client.submit("after", prompt="hello").id).result.content == "still here"
    assert handle.stop() is None
    assert wait_until(lambda: not _pid_alive(pid))
