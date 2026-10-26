"""Sandboxed tools and MCP servers through the server, with the store outside the workspace."""

import json
import os
import shutil
import time
from pathlib import Path

import pytest

from aura_automation import AuraError

from .conftest import AGENT, Workspace, make_workspace

FIXTURES = Path(__file__).parent / "fixtures"
UV = shutil.which("uv")
UV_CACHE = Path(os.environ.get("AURA_TEST_UV_CACHE", "/tmp/aura-e2e/uv-cache"))


def landlock_available() -> bool:
    """Ask the kernel for its Landlock ABI (landlock_create_ruleset with the version flag)."""
    import ctypes
    import platform

    if platform.system() != "Linux" or platform.machine() not in {"x86_64", "aarch64"}:
        return False
    libc = ctypes.CDLL(None, use_errno=True)
    return libc.syscall(444, None, 0, 1) > 0


pytestmark = pytest.mark.skipif(not landlock_available(), reason="Landlock is not available")


def sandbox_for(root: Path, *extra_rw: str, extra_ro: tuple[str, ...] = ()) -> dict:
    """Read the system, the test tree (config, store, fixtures) and the binary; write only the workspace."""
    binary = os.environ.get("AURA_BIN", "/tmp/aura-build/aura")
    ro = ["/usr", "/bin", "/lib", "/lib64", "/etc", str(root), str(Path(binary).parent), str(FIXTURES),
          str(Path.home() / ".local/share/uv"), *extra_ro]
    return {"enabled": True, "restrictions": {"ro": ro, "rw": [".", "/dev/null", *extra_rw]}}


def test_sandboxed_bash_with_external_write_home(tmp_path: Path) -> None:
    ws = make_workspace(tmp_path, sandbox=sandbox_for(tmp_path))
    ws.extra_args.append("--debug")
    outside = tmp_path / "outside"
    outside.mkdir()
    (ws.work / "escape").symlink_to(outside)
    ws.write_script(
        {"calls": [
            {"id": "in", "name": "Bash", "args": {"command": "echo ok > inside.txt && cat inside.txt"}},
            {"id": "out", "name": "Bash", "args": {"command": f"echo bad > {outside}/direct.txt"}},
            {"id": "link", "name": "Bash", "args": {"command": "echo bad > escape/through.txt"}},
        ]},
        {"content": "checked"},
    )
    handle = ws.launch()
    try:
        with handle.client() as client:
            assert client.session().meta.sandbox
            final = client.wait(client.submit("sandbox", prompt="probe").id)
            assert final.outcome == "completed"
            results = {m["tool_call_id"]: m["Content"] for m in client.messages() if m["Role"] == "tool"}
        assert "ok" in results["in"] and (ws.work / "inside.txt").read_text() == "ok\n"
        assert "denied" in results["out"].lower() and "denied" in results["link"].lower()
        assert list(outside.iterdir()) == []
        assert (ws.home / "debug.log").exists(), "debug log belongs to the external write home"
    finally:
        handle.stop()


def mcp_workspace(
    tmp_path: Path, *, required: bool = True, sandbox: bool = True, command: str | None = None, extra_ro: tuple[str, ...] = ()
) -> Workspace:
    agent = AGENT.replace("enabled: [Bash, Read, Write, Ask]", "enabled: [Bash, Read, Write, 'mcp__probe__*']")
    ws = make_workspace(tmp_path, agent=agent, sandbox=sandbox_for(tmp_path, str(UV_CACHE), extra_ro=extra_ro))
    # Trusted server code lives outside the writable workspace.
    trusted = tmp_path / "trusted"
    trusted.mkdir()
    shutil.copy(FIXTURES / "probe_mcp.py", trusted / "probe_mcp.py")
    mcp = ws.config / "config" / "mcp"
    mcp.mkdir(parents=True)
    server = {
        "probe": {
            "type": "stdio",
            "command": command or UV,
            "args": ["run", "--quiet", "--script", str(trusted / "probe_mcp.py")],
            "env": {"UV_CACHE_DIR": "${AURA_TEST_UV_CACHE}"},
            "required": required,
            "sandbox": sandbox,
            "timeout": "120s",
        }
    }
    (mcp / "probe.yaml").write_text(json.dumps(server))
    return ws


@pytest.fixture
def uv_cache(monkeypatch: pytest.MonkeyPatch) -> None:
    UV_CACHE.mkdir(parents=True, exist_ok=True)
    monkeypatch.setenv("AURA_TEST_UV_CACHE", str(UV_CACHE))


def launch_with_cache(ws: Workspace, *, hold: bool = False):
    ws_env = ws.aura.env
    aura = ws.aura
    object.__setattr__(aura, "env", {**ws_env, "AURA_TEST_UV_CACHE": str(UV_CACHE)})
    from aura_automation import Server

    handle = Server.launch(aura, ready_file=ws.root / "ready.json", token="t" * 64, log=ws.root / "serve.log", hold=hold)
    ws.servers.append(handle)
    return handle


@pytest.mark.skipif(UV is None, reason="uv is required for the MCP probe")
def test_sandboxed_mcp_server_is_confined_and_reports_errors(tmp_path: Path, uv_cache: None) -> None:
    ws = mcp_workspace(tmp_path)
    outside = tmp_path / "outside"
    outside.mkdir()
    ws.write_script(
        {"calls": [
            {"id": "in", "name": "mcp__probe__write", "args": {"path": "content.json", "text": "{}"}},
            {"id": "out", "name": "mcp__probe__write", "args": {"path": str(outside / "x.json"), "text": "{}"}},
            {"id": "bad", "name": "mcp__probe__fail", "args": {}},
            {"id": "child", "name": "mcp__probe__spawn", "args": {}},
        ]},
        {"content": "done"},
    )
    handle = launch_with_cache(ws)
    with handle.client() as client:
        info = client.server()
        probe = next(m for m in info.mcp if m.name == "probe")
        assert probe.status == "connected" and probe.required and probe.sandboxed and probe.tools == 5

        run = client.submit("mcp", prompt="use the probe")
        seen = []
        final = client.wait(run.id, on_event=seen.append)
        assert final.outcome == "completed"
        finished = {e.data["call_id"]: e.data for e in seen if e.type == "tool.finished"}
    assert finished["in"]["status"] == "ok" and (ws.work / "content.json").read_text() == "{}"
    assert finished["out"]["status"] == "error" and "denied" in finished["out"]["error"].lower()
    assert finished["bad"]["status"] == "error" and "revision changed" in finished["bad"]["error"]
    assert list(outside.iterdir()) == []
    child = int(json.loads(finished["child"]["result"]) if finished["child"]["result"].startswith("{") else finished["child"]["result"])

    assert handle.stop() == 0
    deadline = time.monotonic() + 10
    while time.monotonic() < deadline:
        try:
            os.kill(child, 0)
        except ProcessLookupError:
            break
        time.sleep(0.1)
    else:
        pytest.fail("the MCP server's child survived the server")


@pytest.mark.skipif(UV is None, reason="uv is required for the MCP probe")
def test_cancelling_a_hanging_mcp_call(tmp_path: Path, uv_cache: None) -> None:
    ws = mcp_workspace(tmp_path)
    ws.write_script({"calls": [{"id": "h", "name": "mcp__probe__hang", "args": {}}]}, {"content": "x", "delay": "60s"})
    handle = launch_with_cache(ws)
    with handle.client() as client:
        run = client.submit("hang", prompt="hang")
        deadline = time.monotonic() + 30
        while time.monotonic() < deadline and not any(e.type == "tool.started" for e in client.journal(0)[0]):
            time.sleep(0.1)
        started = time.monotonic()
        client.cancel(run.id)
        final = client.wait(run.id)
        assert time.monotonic() - started < 20
        assert final.outcome == "cancelled" and final.state == "persisted"
    handle.stop()


def test_required_mcp_failure_prevents_readiness(tmp_path: Path) -> None:
    ws = mcp_workspace(tmp_path, command=str(tmp_path / "no-such-server"))
    with pytest.raises(AuraError) as failure:
        ws.launch()
    assert failure.value.code == "mcp_unavailable"
    ready = json.loads((ws.root / "ready.json").read_text())
    assert "url" not in ready and ready["error"]["code"] == "mcp_unavailable"


def test_unset_variable_in_mcp_config_is_an_error(tmp_path: Path) -> None:
    ws = mcp_workspace(tmp_path)  # AURA_TEST_UV_CACHE is not in the server's environment
    with pytest.raises(AuraError) as failure:
        ws.launch()
    assert failure.value.code == "mcp_unavailable" and "AURA_TEST_UV_CACHE" in failure.value.message


def test_http_mcp_cannot_be_sandboxed(tmp_path: Path) -> None:
    ws = make_workspace(tmp_path)
    mcp = ws.config / "config" / "mcp"
    mcp.mkdir(parents=True)
    (mcp / "remote.yaml").write_text(json.dumps({"remote": {"type": "http", "url": "http://127.0.0.1:1/mcp", "sandbox": True}}))
    with pytest.raises(AuraError) as failure:
        ws.launch()
    assert failure.value.code == "config_invalid" and "cannot be confined" in failure.value.message


@pytest.mark.skipif(UV is None, reason="uv is required for the MCP probe")
def test_children_never_inherit_the_launcher_lease(tmp_path: Path, uv_cache: None) -> None:
    ws = mcp_workspace(tmp_path, extra_ro=("/proc",))
    ws.write_script(
        {"calls": [
            {"id": "mcp", "name": "mcp__probe__fds", "args": {}},
            {"id": "bash", "name": "Bash", "args": {"command": "for f in /proc/self/fd/*; do readlink $f; done"}},
        ]},
        {"content": "listed"},
    )
    handle = launch_with_cache(ws, hold=True)
    lease = f"pipe:[{os.fstat(handle._lease).st_ino}]"

    def server_fds() -> list[str]:
        out = []
        for entry in Path(f"/proc/{handle.pid}/fd").iterdir():
            try:
                out.append(os.readlink(entry))
            except OSError:
                pass
        return out

    assert lease in server_fds(), "the server should hold the lease until released"
    with handle.client() as client:
        seen = []
        client.wait(client.submit("fds", prompt="list descriptors").id, on_event=seen.append)
    finished = {e.data["call_id"]: e.data for e in seen if e.type == "tool.finished"}
    results = {call: data["result"] for call, data in finished.items()}
    assert results["mcp"] and lease not in results["mcp"], f"the MCP server inherited the lease: {finished['mcp']}"
    assert results["bash"] and lease not in results["bash"], "a tool inherited the lease"

    handle.release()
    deadline = time.monotonic() + 5
    while lease in server_fds() and time.monotonic() < deadline:
        time.sleep(0.05)
    assert lease not in server_fds(), "the server kept the lease after release"
    assert handle.stop() == 0
