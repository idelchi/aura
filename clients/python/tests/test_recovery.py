"""Cancellation, typed failures, crash recovery and shutdown settlement."""

import json
import os
import signal
import time

import pytest

from aura_automation import AuraError

from .conftest import Workspace, assert_pairs


def wait_for_event(client, run_id: str, event_type: str, timeout: float = 20.0) -> None:
    deadline = time.monotonic() + timeout
    cursor = 0
    while time.monotonic() < deadline:
        events, _ = client.journal(cursor, wait=1)
        for event in events:
            cursor = event.seq
            if event.type == event_type and event.run_id == run_id:
                return
    pytest.fail(f"{event_type} not observed for {run_id}")


def test_cancel_during_tool_settles_with_complete_pairs(workspace: Workspace) -> None:
    workspace.write_script(
        {"calls": [
            {"id": "slow", "name": "Bash", "args": {"command": "sleep 120"}},
            {"id": "quick", "name": "Bash", "args": {"command": "echo quick"}},
        ]},
        {"content": "unreachable", "delay": "60s"},
    )
    handle = workspace.launch()
    with handle.client() as client:
        run = client.submit("cancel-tool", prompt="work")
        wait_for_event(client, run.id, "tool.started")
        started = time.monotonic()
        assert client.cancel(run.id).cancel_requested
        final = client.wait(run.id)
        assert time.monotonic() - started < 30, "cancellation did not settle promptly"
        assert final.state == "persisted" and final.outcome == "cancelled"
        assert final.error.code == "cancelled" and final.error.details.reason == "request"
        assert not final.error.retryable
        messages = client.messages()
        assert_pairs(messages)
        results = {m["tool_call_id"]: m["Content"] for m in messages if m["Role"] == "tool"}
        assert set(results) == {"slow", "quick"}
        assert "cancel" in results["slow"].lower()
        # Cancelling a settled run changes nothing.
        assert client.cancel(run.id).state == "persisted"
    handle.stop()


def test_cancel_during_stream(workspace: Workspace) -> None:
    workspace.write_script({"content": "partial answer", "stall": "120s"})
    handle = workspace.launch()
    with handle.client() as client:
        run = client.submit("cancel-stream", prompt="answer slowly")
        wait_for_event(client, run.id, "run.started")
        time.sleep(0.5)
        client.cancel(run.id)
        final = client.wait(run.id)
        assert final.outcome == "cancelled" and final.state == "persisted"
        assert_pairs(client.messages())
    handle.stop()


@pytest.mark.parametrize(
    ("failure", "code", "retryable", "status"),
    [
        ({"class": "server", "status": 503, "message": "the engine is not running"}, "provider_server", True, 503),
        ({"class": "network", "message": "connection refused"}, "provider_network", True, None),
        ({"class": "rate_limit", "message": "slow down", "retry_after": "7s"}, "provider_rate_limit", True, 429),
        ({"class": "auth", "message": "bad token"}, "provider_auth", False, 401),
        ({"class": "credit", "message": "no credit"}, "provider_credit", False, 402),
        ({"class": "model_unavailable", "message": "no such model"}, "provider_model_unavailable", False, 404),
        ({"class": "content_filter", "message": "filtered"}, "content_filter", False, None),
    ],
)
def test_provider_failures_are_typed(workspace: Workspace, failure, code, retryable, status) -> None:
    workspace.write_script({"error": failure})
    handle = workspace.launch()
    with handle.client() as client:
        final = client.wait(client.submit("fail", prompt="try").id)
        assert final.state == "persisted" and final.outcome == "failed"
        assert final.error.code == code and final.error.retryable is retryable
        assert final.error.details.status == status
        assert failure["message"] in final.error.message
        if code == "provider_rate_limit":
            assert final.error.details.retry_after_ms == 7000
        # The admitted prompt is kept: the session can be continued later.
        assert client.messages()[0]["Content"] == "try"
    handle.stop()


def test_sigkill_marks_run_lost_and_recovers(workspace: Workspace) -> None:
    workspace.write_script(
        {"calls": [{"id": "first", "name": "Bash", "args": {"command": "echo before"}}]},
        {"calls": [{"id": "hang", "name": "Bash", "args": {"command": "sleep 300"}}]},
        {"content": "recovered"},
    )
    handle = workspace.launch()
    session_id = handle.session_id
    with handle.client() as client:
        run = client.submit("crash", prompt="work then hang")
        wait_for_event(client, run.id, "tool.started")
        deadline = time.monotonic() + 20
        while time.monotonic() < deadline:
            if any(e.type == "tool.started" and e.data["call_id"] == "hang" for e in client.journal(0)[0]):
                break
            time.sleep(0.1)
        last_seq = client.server().last_seq
    handle.kill()

    # A crash can tear the final journal line; the next server repairs it.
    journal = workspace.journal_path(session_id)
    with journal.open("a") as stream:
        stream.write('{"seq": 999, "type": "torn')

    revived = workspace.launch(resume=session_id)
    with revived.client() as client:
        lost = client.run_for("crash")
        assert lost is not None and lost.state == "lost" and lost.terminal
        assert lost.outcome is None and lost.error.code == "server_lost"
        stored = workspace.store.show(session_id)
        assert lost.persisted.revision == stored.revision

        # The first iteration was checkpointed before the kill.
        messages = client.messages()
        assert any(m["Role"] == "tool" and m["tool_call_id"] == "first" for m in messages)
        assert_pairs(messages)

        events, _ = client.journal(last_seq)
        assert [e.seq for e in events] == list(range(last_seq + 1, last_seq + 1 + len(events)))
        types = [e.type for e in events]
        assert "server.started" in types and "run.lost" in types
        assert any(e.type == "notice" and e.data["source"] == "journal" for e in events)

        # Same request is never re-executed; a continuation is a new request.
        assert client.submit("crash", prompt="work then hang").state == "lost"
        resumed = client.wait(client.submit("crash:continue", prompt="continue").id)
        assert resumed.outcome == "completed" and resumed.result.content == "recovered"
    revived.stop()
    for line in journal.read_text().splitlines():
        json.loads(line)


def test_shutdown_settles_the_active_run(workspace: Workspace) -> None:
    workspace.write_script({"calls": [{"id": "hang", "name": "Bash", "args": {"command": "sleep 300"}}]})
    handle = workspace.launch()
    session_id = handle.session_id
    with handle.client() as client:
        run = client.submit("busy", prompt="hang")
        wait_for_event(client, run.id, "tool.started")
    assert handle.stop() == 0

    revived = workspace.launch(resume=session_id)
    with revived.client() as client:
        settled = client.run_for("busy")
        assert settled.state == "persisted" and settled.outcome == "cancelled"
        assert settled.error.details.reason == "shutdown"
        with pytest.raises(AuraError) as stopping:
            revived.stop()
            client.submit("late", prompt="x")
        assert stopping.value.code in {"server_unavailable", "shutting_down"}


def test_sigterm_is_graceful(workspace: Workspace) -> None:
    workspace.write_script({"content": "slow", "stall": "300s"})
    handle = workspace.launch()
    session_id = handle.session_id
    with handle.client() as client:
        run = client.submit("term", prompt="wait")
        wait_for_event(client, run.id, "run.started")
    os.kill(handle.pid, signal.SIGTERM)
    assert handle.process.wait(timeout=60) == 0
    revived = workspace.launch(resume=session_id)
    with revived.client() as client:
        assert client.run_for("term").outcome == "cancelled"
    revived.stop()


def test_second_server_on_a_session_is_refused(workspace: Workspace) -> None:
    handle = workspace.launch()
    with pytest.raises(AuraError) as locked:
        workspace.launch(resume=handle.session_id)
    assert locked.value.code == "session_locked"
    assert handle.alive()
    handle.stop()
