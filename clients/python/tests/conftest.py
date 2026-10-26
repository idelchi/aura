"""End-to-end fixtures: real `aura` binary, deterministic script provider, temp workspaces."""

import json
import os
import secrets
import shutil
from collections.abc import Iterator
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

import pytest

from aura_automation import Aura, Server, ServerHandle, Store

BINARY = Path(os.environ.get("AURA_BIN", "/tmp/aura-build/aura"))


@dataclass
class Workspace:
    """A workspace with its own config and a write home (session store) outside it."""

    root: Path
    work: Path
    config: Path
    home: Path
    script: Path
    servers: list[ServerHandle] = field(default_factory=list)
    extra_args: list[str] = field(default_factory=list)

    @property
    def aura(self) -> Aura:
        env = {
            "PATH": os.environ["PATH"],
            "HOME": str(self.root),
            "TMPDIR": str(self.work / ".tmp"),
        }
        args = [
            "--home", "", "--env-file", "/dev/null",
            "--config", str(self.config), "--config", str(self.home),
            "--without-plugins", *self.extra_args,
        ]
        return Aura(binary=BINARY, args=args, cwd=self.work, env=env)

    @property
    def store(self) -> Store:
        return Store(self.aura)

    def write_script(self, *steps: dict[str, Any]) -> None:
        self.script.write_text(json.dumps({"models": [{"name": "script", "context": 65536}], "steps": list(steps)}))
        Path(str(self.script) + ".cursor").unlink(missing_ok=True)

    def launch(self, *, resume: str | None = None, token: str | None = None) -> ServerHandle:
        handle = Server.launch(
            self.aura,
            ready_file=self.root / "ready.json",
            token=token or TOKEN,
            resume=resume,
            log=self.root / "serve.log",
        )
        self.servers.append(handle)
        return handle

    def journal_path(self, session_id: str) -> Path:
        return self.home / "sessions" / "journal" / f"{session_id}.jsonl"

    def session_path(self, session_id: str) -> Path:
        return self.home / "sessions" / f"{session_id}.json"


TOKEN = secrets.token_hex(32)

AGENT = """---
name: Scripted
default: true
model:
  provider: scripted
  name: script
  context: 65536
tools:
  enabled: [Bash, Read, Write, Ask]
---
Scripted agent.
"""


def make_workspace(root: Path, *, agent: str = AGENT, sandbox: dict[str, Any] | None = None) -> Workspace:
    work, config, home = root / "work", root / "config", root / "home"
    for path in (work / ".tmp", config / "config" / "agents", config / "config" / "providers", home):
        path.mkdir(parents=True, exist_ok=True)
    script = root / "script.json"
    (config / "config" / "providers" / "script.yaml").write_text(
        f"scripted:\n  type: script\n  script: {script}\n"
    )
    (config / "config" / "agents" / "scripted.md").write_text(agent)
    features = config / "config" / "features"
    features.mkdir()
    (features / "tools.yaml").write_text("tools:\n  max_steps: -1\n  opt_in: []\n")
    (features / "title.yaml").write_text("title:\n  disabled: true\n")
    (features / "snapshot.yaml").write_text("snapshot:\n  disabled: true\n")
    if sandbox is not None:
        (features / "sandbox.yaml").write_text("sandbox:\n" + _yaml(sandbox, 2))
    ws = Workspace(root=root, work=work, config=config, home=home, script=script)
    ws.write_script({"content": "default"})
    return ws


def _yaml(value: Any, indent: int) -> str:
    pad = " " * indent
    if isinstance(value, dict):
        out = ""
        for key, item in value.items():
            if isinstance(item, (dict, list)):
                out += f"{pad}{key}:\n" + _yaml(item, indent + 2)
            else:
                out += f"{pad}{key}: {json.dumps(item)}\n"
        return out
    return "".join(f"{pad}- {json.dumps(item)}\n" for item in value)


@pytest.fixture
def workspace(tmp_path: Path) -> Iterator[Workspace]:
    if not BINARY.exists():
        pytest.fail(f"aura binary not found at {BINARY}; build it or set AURA_BIN")
    ws = make_workspace(tmp_path)
    yield ws
    for handle in ws.servers:
        if handle.alive():
            handle.kill()


def assert_pairs(messages: list[dict[str, Any]]) -> None:
    """Every assistant call has exactly one later result and every result answers a call."""
    open_calls: set[str] = set()
    for message in messages:
        for call in message.get("calls") or []:
            open_calls.add(call["id"])
        if message["Role"] == "tool":
            assert message["tool_call_id"] in open_calls, f"orphan result {message['tool_call_id']}"
            open_calls.remove(message["tool_call_id"])
    assert not open_calls, f"calls without results: {open_calls}"


def copy_tree(source: Path, target: Path) -> None:
    shutil.copytree(source, target)
