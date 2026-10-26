"""Session store operations and independent forks of an immutable parent."""

import hashlib
import json
import threading
from pathlib import Path

import pytest

from aura_automation import AuraError

from .conftest import Workspace, make_workspace


def seed_parent(workspace: Workspace) -> str:
    """Create a conversation through the server and return its session ID."""
    workspace.write_script({"content": "lesson one"})
    handle = workspace.launch()
    with handle.client() as client:
        client.wait(client.submit("teach", prompt="teach me").id)
    handle.stop()
    return handle.session_id


def test_import_export_freeze_idempotent(workspace: Workspace, tmp_path: Path) -> None:
    parent = seed_parent(workspace)
    exported = tmp_path / "native.json"
    result = workspace.store.export(parent, exported)
    assert result.written and result.sha256 == hashlib.sha256(exported.read_bytes()).hexdigest()
    assert not workspace.store.export(parent, exported).written

    other = make_workspace(tmp_path / "other")
    first = other.store.import_file(exported, freeze=True)
    assert first.created and first.frozen_now and first.session.frozen
    assert first.source_sha256 == result.sha256
    again = other.store.import_file(exported, freeze=True)
    assert not again.created and not again.frozen_now and again.session.revision == first.session.revision

    with pytest.raises(AuraError) as frozen:
        other.launch(resume=parent)
    assert frozen.value.code == "session_frozen"

    tampered = json.loads(exported.read_text())
    tampered["title"] = "different"
    exported.write_text(json.dumps(tampered))
    with pytest.raises(AuraError) as exists:
        other.store.import_file(exported)
    assert exists.value.code == "exists"

    with pytest.raises(AuraError) as missing:
        other.store.show("nope")
    assert missing.value.code == "not_found"

    listing = other.store.list()
    assert [s.id for s in listing.sessions] == [parent] and listing.warnings == []


def test_parallel_forks_are_independent(workspace: Workspace, tmp_path: Path) -> None:
    parent = seed_parent(workspace)
    frozen = workspace.store.freeze(parent)
    assert frozen.changed and workspace.store.freeze(parent).changed is False
    parent_bytes = workspace.session_path(parent).read_bytes()

    forks = []
    for name in ("left", "right"):
        child = workspace.store.fork(parent, f"task-{name}", expect_revision=frozen.session.revision)
        assert child.created and child.session.parent.revision == frozen.session.revision
        assert child.session.messages == frozen.session.messages
        assert not workspace.store.fork(parent, f"task-{name}").created
        # Each fork runs in its own workspace and config, sharing only the store.
        ws = make_workspace(tmp_path / name)
        ws.home = workspace.home
        ws.write_script({"content": f"{name} answer"})
        forks.append((ws, child.session.id))

    handles = [ws.launch(resume=child) for ws, child in forks]
    results: dict[str, object] = {}

    def drive(handle, name: str) -> None:
        with handle.client() as client:
            results[name] = client.wait(client.submit(f"{name}:1", prompt=f"{name} work").id)

    threads = [threading.Thread(target=drive, args=(h, n)) for h, n in zip(handles, ("left", "right"))]
    for thread in threads:
        thread.start()
    for thread in threads:
        thread.join(60)
    for handle in handles:
        handle.stop()

    assert {n: r.result.content for n, r in results.items()} == {"left": "left answer", "right": "right answer"}
    assert workspace.session_path(parent).read_bytes() == parent_bytes, "parent changed"
    left, right = (json.loads(workspace.session_path(c).read_text()) for _, c in forks)
    assert left["messages"][-1]["Content"] == "left answer" and right["messages"][-1]["Content"] == "right answer"
    assert workspace.journal_path("task-left").exists() and workspace.journal_path("task-right").exists()
    assert not workspace.journal_path(parent).read_text().count("left work")

    with pytest.raises(AuraError) as stale:
        workspace.store.fork(parent, "task-stale", expect_revision="0" * 64)
    assert stale.value.code == "revision_mismatch"
