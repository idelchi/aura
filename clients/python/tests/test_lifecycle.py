"""Run lifecycle over the real server: acceptance, persistence, idempotency, commands."""

import hashlib
import json

import pytest

from aura_automation import AuraError, StopRequested

from .conftest import Workspace, assert_pairs


def test_fresh_session_is_saved_before_ready(workspace: Workspace) -> None:
    handle = workspace.launch()
    stored = workspace.session_path(handle.session_id)
    assert stored.exists(), "fresh session not saved before readiness"
    with handle.client() as client:
        session = client.session()
        assert session.id == handle.session_id
        assert session.revision == hashlib.sha256(stored.read_bytes()).hexdigest()
        assert session.messages == 0
    assert handle.stop() == 0


def test_run_lifecycle_and_journal_order(workspace: Workspace) -> None:
    workspace.write_script(
        {"content": "Looking.", "thinking": "SECRET-THOUGHT", "calls": [{"id": "c1", "name": "Bash", "args": {"command": "echo tool-ran"}}]},
        {"content": "All done."},
    )
    handle = workspace.launch()
    with handle.client() as client:
        accepted = client.submit("task:author:1", prompt="Do the task")
        assert accepted.state == "accepted" and not accepted.terminal
        seen = []
        final = client.wait(accepted.id, on_event=seen.append)

        assert final.state == "persisted" and final.terminal
        assert final.outcome == "completed" and final.error is None and final.persist_error is None
        assert final.result is not None and final.result.content == "All done."
        assert final.stats.tool_calls == {"Bash": 1} and final.stats.iterations == 2

        # Durable events of the run, in causal order.
        types = [e.type for e in seen if e.run_id == accepted.id and e.type != "status"]
        assert types == [
            "run.accepted", "message.user", "run.started", "message.assistant", "tool.started",
            "tool.finished", "session.persisted", "message.assistant", "run.finished",
            "session.persisted", "run.persisted",
        ]
        first_reply = next(e for e in seen if e.type == "message.assistant")
        assert first_reply.data["tool_calls"][0]["call_id"] == "c1"
        finished = next(e for e in seen if e.type == "tool.finished")
        assert finished.data["status"] == "ok" and "tool-ran" in finished.data["result"]
        checkpoint = next(e for e in seen if e.type == "session.persisted" and e.run_id == accepted.id)
        assert checkpoint.data["reason"] == "checkpoint"

        # Persisted only after the save: the revision is the stored file's digest.
        stored = workspace.session_path(handle.session_id).read_bytes()
        assert final.persisted is not None
        assert final.persisted.revision == hashlib.sha256(stored).hexdigest()
        assert final.persisted.seq == next(e.seq for e in seen if e.type == "run.persisted") - 1

        # The journal never contains thinking; messages omit it unless asked.
        assert "SECRET-THOUGHT" not in workspace.journal_path(handle.session_id).read_text()
        assert "SECRET-THOUGHT" not in json.dumps(client.messages())
        assert "SECRET-THOUGHT" in json.dumps(client.messages(thinking=True))
        assert_pairs(client.messages())
    assert handle.stop() == 0


def test_request_idempotency_and_conflicts(workspace: Workspace) -> None:
    workspace.write_script({"content": "one"}, {"content": "two"})
    handle = workspace.launch()
    with handle.client() as client:
        first = client.submit("req-1", prompt="hello")
        final = client.wait(first.id)
        again = client.submit("req-1", prompt="hello")
        assert again.id == first.id and again.state == "persisted"
        assert client.run_for("req-1") == again

        with pytest.raises(AuraError) as conflict:
            client.submit("req-1", prompt="something else")
        assert conflict.value.code == "request_conflict" and conflict.value.status == 409

        with pytest.raises(AuraError) as empty:
            client.submit("req-2", prompt="   ")
        assert empty.value.code == "bad_request"

        assert client.run_for("never-sent") is None
        second = client.submit("req-2", prompt="again")
        assert client.wait(second.id).result.content == "two"
        assert final.result.content == "one"
    handle.stop()


def test_busy_while_a_run_is_active(workspace: Workspace) -> None:
    workspace.write_script({"content": "slow", "stall": "3s"}, {"content": "next"})
    handle = workspace.launch()
    with handle.client() as client:
        run = client.submit("a", prompt="first")
        with pytest.raises(AuraError) as busy:
            client.submit("b", prompt="second")
        assert busy.value.code == "busy"
        assert client.wait(run.id).outcome == "completed"
        assert client.wait(client.submit("b", prompt="second").id).outcome == "completed"
    handle.stop()


def test_prompts_are_never_commands(workspace: Workspace) -> None:
    workspace.write_script({"content": "treated as text"})
    handle = workspace.launch()
    with handle.client() as client:
        run = client.wait(client.submit("p", prompt="/exit is just text").id)
        assert run.outcome == "completed"
        assert client.messages()[0]["Content"] == "/exit is just text"
        assert handle.alive()
    handle.stop()


def test_commands(workspace: Workspace) -> None:
    handle = workspace.launch()
    with handle.client() as client:
        ok = client.wait(client.submit("c1", command="/stats").id)
        assert ok.kind == "command" and ok.outcome == "completed" and ok.state == "persisted"
        assert ok.result is not None and ok.result.content

        unknown = client.wait(client.submit("c2", command="/no-such-command").id)
        assert unknown.outcome == "failed" and unknown.error.code == "command_failed"

        for rebinding in ("/resume other", "/fork", "/clear", "/new", "/name x", "/exit", "/quit", "/load x"):
            with pytest.raises(AuraError) as rejected:
                client.submit("r" + rebinding, command=rebinding)
            assert rejected.value.code == "bad_request", rebinding

        with pytest.raises(AuraError) as bare:
            client.submit("c3", command="stats")
        assert bare.value.code == "bad_request"
    handle.stop()


def test_wait_stop_does_not_cancel(workspace: Workspace) -> None:
    workspace.write_script({"content": "late", "stall": "4s"})
    handle = workspace.launch()
    with handle.client() as client:
        run = client.submit("w", prompt="go")
        calls = []
        with pytest.raises(StopRequested):
            client.wait(run.id, stop=lambda: calls.append(1) or len(calls) > 1, poll=1)
        assert not client.run(run.id).cancel_requested
        assert client.wait(run.id).outcome == "completed"
    handle.stop()


def test_unauthorized(workspace: Workspace) -> None:
    from aura_automation import Client

    handle = workspace.launch()
    with Client(handle.url, "x" * 64) as stranger, pytest.raises(AuraError) as denied:
        stranger.server()
    assert denied.value.code == "unauthorized" and denied.value.status == 401
    handle.stop()


def test_events_stop_while_the_stream_is_quiet(workspace: Workspace) -> None:
    import time

    handle = workspace.launch()
    with handle.client() as client:
        last = client.server().last_seq
        deadline = time.monotonic() + 2
        started = time.monotonic()
        with pytest.raises(StopRequested):
            for _ in client.events(last, stop=lambda: time.monotonic() > deadline, heartbeat=1):
                pass
        assert time.monotonic() - started < 6
    handle.stop()


def test_startup_metadata_failure_is_typed(workspace: Workspace) -> None:
    import json as jsonlib

    workspace.write_script({"content": "ok"})
    handle = workspace.launch()
    session_id = handle.session_id
    handle.stop()

    # Restoring a model contacts the provider; an outage there must stay typed and retryable.
    script = jsonlib.loads(workspace.script.read_text())
    script["metadata_error"] = {"class": "server", "status": 503, "message": "the engine is not running"}
    workspace.script.write_text(jsonlib.dumps(script))
    workspace.extra_args += ["--model", "script"]
    with pytest.raises(AuraError) as outage:
        workspace.launch(resume=session_id)
    assert outage.value.code == "provider_server" and outage.value.retryable
    assert outage.value.details["status"] == 503 and "the engine is not running" in outage.value.message

    script["metadata_error"] = {"class": "auth", "message": "invalid token"}
    workspace.script.write_text(jsonlib.dumps(script))
    with pytest.raises(AuraError) as credentials:
        workspace.launch(resume=session_id)
    assert credentials.value.code == "provider_auth" and not credentials.value.retryable


def test_launch_reaps_a_server_with_an_invalid_ready_file(workspace: Workspace, tmp_path) -> None:
    import os
    import stat

    from aura_automation import Aura, Server

    fake = tmp_path / "fake-aura"
    fake.write_text(
        "#!/bin/sh\n"
        'for a in "$@"; do last="$prev"; prev="$a"; done\n'
        'printf \'{"api":1,"url":"http://127.0.0.1:1","pid":1,"session_id":"x","bogus":true}\' > "$prev"\n'
        "echo $$ > " + str(tmp_path / "fake.pid") + "\n"
        "exec sleep 300\n"
    )
    fake.chmod(fake.stat().st_mode | stat.S_IXUSR)
    aura = Aura(binary=fake, cwd=tmp_path, env={"PATH": os.environ["PATH"]})
    with pytest.raises(AuraError) as mismatch:
        Server.launch(aura, ready_file=tmp_path / "ready.json", token="t" * 64)
    assert mismatch.value.code == "contract_mismatch"
    pid = int((tmp_path / "fake.pid").read_text())
    with pytest.raises(ProcessLookupError):
        os.kill(pid, 0)


def test_attached_handle_treats_zombies_as_dead(tmp_path) -> None:
    import subprocess
    import time

    from aura_automation.models import Ready
    from aura_automation.server import ServerHandle

    child = subprocess.Popen(["true"])
    time.sleep(0.5)  # exited, deliberately not reaped yet: a zombie
    handle = ServerHandle(Ready(api=1, url="http://127.0.0.1:1", pid=child.pid, session_id="s"), "t" * 64, None)
    assert not handle.alive()
    child.wait()
