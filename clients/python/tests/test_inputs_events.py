"""Questions and approvals under both input policies; SSE replay and transient events."""

import threading
import time

from aura_automation.models import Event, Transient

from .conftest import Workspace

ASK = {"calls": [{"id": "q", "name": "Ask", "args": {"question": "Which colour?", "options": [
    {"label": "red", "description": "warm"}, {"label": "blue", "description": "cold"}]}}]}


def test_block_policy_ends_the_run_blocked(workspace: Workspace) -> None:
    workspace.write_script(ASK, {"content": "never", "delay": "30s"})
    handle = workspace.launch()
    with handle.client() as client:
        final = client.wait(client.submit("ask", prompt="choose").id)
        assert final.outcome == "blocked" and final.error.code == "input_required"
        assert final.state == "persisted"
    handle.stop()


def test_wait_policy_publishes_and_accepts_answers(workspace: Workspace) -> None:
    workspace.write_script(ASK, {"content": "you chose"})
    handle = workspace.launch()
    with handle.client() as client:
        run = client.submit("ask", prompt="choose", on_input="wait")
        answered = []

        def answer(event: Event) -> None:
            if event.type == "input.required":
                assert event.data["kind"] == "ask" and event.data["question"] == "Which colour?"
                assert [o["label"] for o in event.data["options"]] == ["red", "blue"]
                client.answer(run.id, event.data["input_id"], answer="2")
                answered.append(event.data["input_id"])

        final = client.wait(run.id, on_event=answer)
        assert answered and final.outcome == "completed"
        tool = next(m for m in client.messages() if m["Role"] == "tool")
        assert "blue" in tool["Content"]
        resolved = [e for e in client.journal(0)[0] if e.type == "input.resolved"]
        assert resolved[0].data == {"input_id": answered[0], "answer": "blue"}
    handle.stop()


def test_sse_replays_and_reconnects_by_cursor(workspace: Workspace) -> None:
    workspace.write_script({"content": "streamed text", "thinking": "SECRET-THOUGHT"}, {"content": "second"})
    handle = workspace.launch()
    with handle.client() as client:
        first = client.wait(client.submit("one", prompt="go").id)
        events = []
        for event in client.events(0):
            events.append(event)
            if event.type == "run.persisted":
                break
        assert [e.seq for e in events] == list(range(1, len(events) + 1))
        assert all(isinstance(e, Event) for e in events)

        # Resume from the middle: nothing before the cursor, nothing skipped after it.
        cursor = events[len(events) // 2].seq
        tail = []
        for event in client.events(cursor):
            tail.append(event)
            if event.type == "run.persisted":
                break
        assert [e.seq for e in tail] == list(range(cursor + 1, events[-1].seq + 1))

        # Transient deltas stream alongside, thinking only when asked.
        received: list[Transient | Event] = []
        stop = threading.Event()

        def listen() -> None:
            for item in client.events(client.server().last_seq, transient=True, stop=stop.is_set):
                received.append(item)
                if isinstance(item, Event) and item.type == "run.persisted":
                    return

        listener = threading.Thread(target=listen)
        listener.start()
        time.sleep(0.5)
        client.wait(client.submit("two", prompt="again").id)
        listener.join(30)
        deltas = [t for t in received if isinstance(t, Transient)]
        assert any(t.type == "delta.content" and "second" in t.data["text"] for t in deltas)
        assert not any(t.type == "delta.thinking" for t in deltas)
        assert first.outcome == "completed"
    handle.stop()
