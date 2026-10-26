"""HTTP client of the ``aura serve`` /v1 API."""

import time
from collections.abc import Callable, Iterator
from typing import Any

import httpx
from httpx_sse import connect_sse
from pydantic import BaseModel, ValidationError

from .errors import AuraError, StopRequested
from .models import (
    HEARTBEAT,
    TERMINAL_EVENTS,
    TRANSIENT_EVENTS,
    ErrorInfo,
    Event,
    InputPolicy,
    Journal,
    Run,
    ServerInfo,
    SessionInfo,
    Transient,
)

#: Transport failures of idempotent requests are retried this many times.
TRANSPORT_RETRIES = 3


class Client:
    """Client of one served session."""

    def __init__(self, url: str, token: str, *, timeout: float = 30.0) -> None:
        self.url = url.rstrip("/")
        self.timeout = timeout
        self._http = httpx.Client(
            base_url=self.url,
            headers={"Authorization": f"Bearer {token}"},
            timeout=timeout,
        )

    def close(self) -> None:
        self._http.close()

    def __enter__(self) -> "Client":
        return self

    def __exit__(self, *_: object) -> None:
        self.close()

    def server(self) -> ServerInfo:
        return self._get(ServerInfo, "/v1/server")

    def session(self) -> SessionInfo:
        return self._get(SessionInfo, "/v1/session")

    def messages(self, after: int = 0, *, thinking: bool = False) -> list[dict[str, Any]]:
        """Stored non-system messages from index ``after`` (thinking omitted unless asked)."""
        data = self._json("GET", "/v1/session/messages", params={"after": after, "thinking": _flag(thinking)})
        return data["messages"]

    def submit(
        self,
        request_id: str,
        *,
        prompt: str | None = None,
        command: str | None = None,
        on_input: InputPolicy = "block",
    ) -> Run:
        """Submit a run. Resubmitting the same request (any state) returns the same run."""
        if (prompt is None) == (command is None):
            raise ValueError("pass exactly one of prompt and command")
        body: dict[str, Any] = {"request_id": request_id, "on_input": on_input}
        if prompt is not None:
            body["prompt"] = prompt
        else:
            body["command"] = command
        return Run.model_validate(self._json("POST", "/v1/runs", json=body))

    def run(self, run_id: str) -> Run:
        return self._get(Run, f"/v1/runs/{run_id}")

    def run_for(self, request_id: str) -> Run | None:
        """The run of a request ID, or None if it was never accepted."""
        try:
            return self._get(Run, "/v1/runs", params={"request_id": request_id})
        except AuraError as error:
            if error.code == "not_found":
                return None
            raise

    def cancel(self, run_id: str) -> Run:
        """Request cancellation (idempotent). The settled record is authoritative."""
        return Run.model_validate(self._json("POST", f"/v1/runs/{run_id}/cancel"))

    def answer(self, run_id: str, input_id: str, *, answer: str | None = None, action: str | None = None) -> None:
        """Answer an ask (``answer``) or a confirm (``action``: allow, allow_session, deny)."""
        if (answer is None) == (action is None):
            raise ValueError("pass exactly one of answer and action")
        body = {"answer": answer} if answer is not None else {"action": action}
        self._json("POST", f"/v1/runs/{run_id}/inputs/{input_id}", json=body, retry=False)

    def journal(self, after: int = 0, *, limit: int = 500, wait: float = 0) -> tuple[list[Event], int]:
        """Durable events with seq > ``after``, long-polling up to ``wait`` seconds; returns (events, last_seq)."""
        data = self._json(
            "GET",
            "/v1/journal",
            params={"after": after, "limit": limit, "wait": int(wait)},
            timeout=self.timeout + wait,
        )
        page = Journal.model_validate(data)
        return page.events, page.last_seq

    def events(
        self,
        after: int = 0,
        *,
        transient: bool = False,
        thinking: bool = False,
        stop: Callable[[], bool] | None = None,
        heartbeat: int = 15,
    ) -> Iterator[Event | Transient]:
        """Stream events over SSE from ``after``, reconnecting from the last seen seq.

        The server sends a heartbeat every ``heartbeat`` seconds on a quiet stream, so
        ``stop`` is honoured within that interval; raises StopRequested. A stream silent
        for three heartbeats is treated as dead and reopened.
        """
        cursor = after
        params = {"transient": _flag(transient), "thinking": _flag(thinking), "heartbeat": heartbeat}
        timeout = httpx.Timeout(self.timeout, read=3 * heartbeat)
        while True:
            if stop and stop():
                raise StopRequested(cursor)
            try:
                headers = {"Last-Event-ID": str(cursor)}
                with connect_sse(self._http, "GET", "/v1/events", params=params, headers=headers, timeout=timeout) as source:
                    if source.response.status_code != 200:
                        source.response.read()
                        raise _response_error(source.response)
                    for sse in source.iter_sse():
                        if stop and stop():
                            raise StopRequested(cursor)
                        if sse.event == HEARTBEAT:
                            continue
                        # The SSE last-event-ID is sticky, so transient events are told apart by type.
                        if sse.event in TRANSIENT_EVENTS:
                            yield Transient(type=sse.event, data=sse.json())
                        else:
                            event = Event.model_validate_json(sse.data)
                            cursor = event.seq
                            yield event
            except httpx.TransportError:
                time.sleep(1)

    def wait(
        self,
        run_id: str,
        *,
        after: int = 0,
        on_event: Callable[[Event], None] | None = None,
        stop: Callable[[], bool] | None = None,
        poll: float = 10.0,
    ) -> Run:
        """Wait until the run is terminal, calling ``on_event`` for each durable event after ``after``.

        Long-polls the journal, so ``stop`` is honoured within ``poll`` seconds; raises
        StopRequested (the run keeps going). Returns the terminal run record.
        """
        cursor = after
        while True:
            if stop and stop():
                raise StopRequested(cursor)
            events, _ = self.journal(cursor, wait=poll)
            for event in events:
                cursor = event.seq
                if on_event:
                    on_event(event)
                if event.type in TERMINAL_EVENTS and event.run_id == run_id:
                    return event.run()
            if not events:
                current = self.run(run_id)
                if current.terminal:
                    return current

    def shutdown(self) -> None:
        """Ask the server to settle its active run and exit."""
        self._json("POST", "/v1/shutdown", retry=False)

    def _get[T: BaseModel](self, model: type[T], path: str, *, params: dict[str, Any] | None = None) -> T:
        try:
            return model.model_validate(self._json("GET", path, params=params))
        except ValidationError as error:
            raise AuraError("contract_mismatch", f"unexpected response from {path}: {error}") from error

    def _json(
        self,
        method: str,
        path: str,
        *,
        json: Any = None,
        params: dict[str, Any] | None = None,
        timeout: float | None = None,
        retry: bool = True,
    ) -> Any:
        attempts = TRANSPORT_RETRIES if retry else 1
        for attempt in range(attempts):
            try:
                response = self._http.request(
                    method, path, json=json, params=params, timeout=timeout or self.timeout
                )
                break
            except httpx.TransportError as error:
                if attempt == attempts - 1:
                    raise AuraError("server_unavailable", f"{method} {path}: {error}") from error
                time.sleep(1)
        if response.status_code >= 400:
            raise _response_error(response)
        return response.json()


def _response_error(response: httpx.Response) -> AuraError:
    try:
        info = ErrorInfo.model_validate(response.json()["error"])
    except (ValueError, KeyError, TypeError, ValidationError):
        return AuraError("http_error", f"HTTP {response.status_code}: {response.text[:500]}", status=response.status_code)
    return AuraError.from_info(info, status=response.status_code)


def _flag(value: bool) -> str:
    return "true" if value else "false"
