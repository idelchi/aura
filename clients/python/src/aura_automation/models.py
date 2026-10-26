"""Strict models of Aura's automation contract (API version 1)."""

from datetime import datetime
from typing import Any, Literal

from pydantic import BaseModel, ConfigDict

API_VERSION = 1

RunState = Literal["accepted", "running", "finished", "persisted", "persist_failed", "lost"]
Outcome = Literal["completed", "failed", "cancelled", "rejected", "blocked"]
InputPolicy = Literal["block", "wait"]

#: Event types that end a run: the event's data carries the terminal run record.
TERMINAL_EVENTS = frozenset({"run.persisted", "run.persist_failed", "run.lost"})

#: SSE event sent on a quiet stream; neither journaled nor yielded to callers.
HEARTBEAT = "heartbeat"

#: Unsequenced live event types (only streamed with ``transient=True``; may be dropped).
TRANSIENT_EVENTS = frozenset({"delta.content", "delta.thinking", "tool.output", "spinner"})


class Model(BaseModel):
    """Contract model: unknown fields are an error, instances are immutable."""

    model_config = ConfigDict(extra="forbid", frozen=True)


class Details(Model):
    status: int | None = None
    provider: str | None = None
    retry_after_ms: int | None = None
    reason: str | None = None
    last_seq: int | None = None


class ErrorInfo(Model):
    code: str
    message: str
    retryable: bool
    details: Details


class Lineage(Model):
    id: str
    revision: str


class SessionSummary(Model):
    id: str
    title: str
    created_at: datetime
    updated_at: datetime
    revision: str
    messages: int
    frozen: bool
    parent: Lineage | None


class SessionList(Model):
    sessions: list[SessionSummary]
    warnings: list[str]


class ForkResult(Model):
    session: SessionSummary
    parent: SessionSummary
    created: bool


class FreezeResult(Model):
    session: SessionSummary
    changed: bool


class ImportResult(Model):
    session: SessionSummary
    created: bool
    frozen_now: bool
    source_sha256: str


class ExportResult(Model):
    session: SessionSummary
    path: str
    sha256: str
    written: bool


class Persisted(Model):
    revision: str
    messages: int
    seq: int


class Stats(Model):
    iterations: int
    tool_calls: dict[str, int]
    tool_errors: int


class Result(Model):
    content: str


class Run(Model):
    id: str
    request_id: str
    kind: Literal["prompt", "command"]
    request_digest: str
    on_input: InputPolicy
    session_id: str
    state: RunState
    terminal: bool
    outcome: Outcome | None
    error: ErrorInfo | None
    persist_error: ErrorInfo | None
    cancel_requested: bool
    result: Result | None
    persisted: Persisted | None
    stats: Stats
    accepted_seq: int
    accepted_at: datetime
    started_at: datetime | None
    finished_at: datetime | None
    settled_at: datetime | None


class Event(Model):
    """A durable journal event. ``data`` follows the event type (see the contract)."""

    seq: int
    time: datetime
    type: str
    session_id: str
    run_id: str | None
    data: dict[str, Any]

    def run(self) -> Run:
        """The run record carried by ``run.*`` events."""
        return Run.model_validate(self.data["run"])


class Transient(Model):
    """An unsequenced live event (content/thinking deltas, tool output, spinner); may be dropped."""

    type: str
    data: dict[str, Any]


class MCPServer(Model):
    name: str
    status: Literal["connected", "failed"]
    required: bool
    sandboxed: bool
    tools: int
    error: str


class ServerInfo(Model):
    api: int
    version: str
    commit: str
    pid: int
    session_id: str
    workdir: str
    write_home: str
    active_run: str | None
    last_seq: int
    shutting_down: bool
    mcp: list[MCPServer]


class SessionMeta(Model):
    agent: str
    mode: str
    model: str
    provider: str
    think: str
    sandbox: bool


class SessionStatus(Model):
    iteration: int
    tokens_used: int
    tokens_max: int


class SessionInfo(SessionSummary):
    persisted_seq: int
    meta: SessionMeta
    status: SessionStatus


class Ready(Model):
    """Content of the serve ready file: the address, or the startup error."""

    api: int
    url: str | None = None
    pid: int | None = None
    session_id: str | None = None
    aura_version: str | None = None
    commit: str | None = None
    workdir: str | None = None
    write_home: str | None = None
    error: ErrorInfo | None = None


class Journal(Model):
    events: list[Event]
    last_seq: int
