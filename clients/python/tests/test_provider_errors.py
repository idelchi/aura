"""Provider failures through the public API, using the real llama-swap provider against a fake server.

A stopped engine (503) and rate limits (429) must reach clients as retryable provider errors, an
authenticating proxy's 401/403 HTML as a credential failure, and an unreachable server as a network
failure — never as `internal`.
"""

import json
import threading
from collections.abc import Iterator
from dataclasses import dataclass, field
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

import pytest

from aura_automation import AuraError

from .conftest import Workspace, make_workspace

LOGIN_PAGE = "<!DOCTYPE html><html><head><title>Login - Authelia</title></head><body>Sign in</body></html>"
ENGINE_STOPPED = '{"error":{"code":503,"message":"the engine is not running","type":"unavailable_error"}}'
RATE_LIMITED = '{"error":{"code":429,"message":"too many requests","type":"rate_limit_error"}}'
PROPS = json.dumps({
    "default_generation_settings": {"n_ctx": 32768},
    "chat_template_caps": {"supports_tool_calls": True},
    "chat_template": "",
    "modalities": {"vision": False},
})


@dataclass
class Reply:
    status: int
    content_type: str
    body: str
    headers: dict[str, str] = field(default_factory=dict)


OK_PROPS = Reply(200, "application/json", PROPS)


class FakeLlamaSwap(ThreadingHTTPServer):
    """Answers llama-swap's metadata route and the chat completions route with set replies."""

    def __init__(self) -> None:
        self.props = OK_PROPS
        self.chat = Reply(500, "application/json", '{"error":{"message":"unset","type":"server_error"}}')
        self.paths: list[str] = []
        super().__init__(("127.0.0.1", 0), Handler)

    @property
    def url(self) -> str:
        return f"http://127.0.0.1:{self.server_address[1]}"


class Handler(BaseHTTPRequestHandler):
    server: FakeLlamaSwap

    def _reply(self) -> None:
        path = self.path.split("?")[0]
        self.server.paths.append(path)
        length = int(self.headers.get("Content-Length") or 0)
        self.rfile.read(length)
        reply = self.server.props if path == "/upstream/m/props" else self.server.chat
        body = reply.body.encode()
        self.send_response(reply.status)
        self.send_header("Content-Type", reply.content_type)
        self.send_header("Content-Length", str(len(body)))
        for key, value in reply.headers.items():
            self.send_header(key, value)
        self.end_headers()
        self.wfile.write(body)

    do_GET = do_POST = _reply

    def log_message(self, *_: object) -> None:
        pass


@pytest.fixture
def fake() -> Iterator[FakeLlamaSwap]:
    server = FakeLlamaSwap()
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    yield server
    server.shutdown()
    server.server_close()


AGENT = """---
name: Swapped
default: true
model:
  provider: swap
  name: m
  context: 32768
tools:
  enabled: [Bash]
---
Agent behind llama-swap.
"""


def swap_workspace(tmp_path: Path, url: str) -> Workspace:
    ws = make_workspace(tmp_path, agent=AGENT)
    (ws.config / "config" / "providers" / "swap.yaml").write_text(f"swap:\n  type: llamaswap\n  url: {url}\n  timeout: 30s\n")
    return ws


def failed_run(ws: Workspace):
    handle = ws.launch()
    with handle.client() as client:
        run = client.wait(client.submit("probe", prompt="hello").id)
    assert handle.stop() == 0
    assert run.state == "persisted" and run.outcome == "failed", run
    return run.error


@pytest.mark.parametrize(
    ("reply", "code", "retryable", "text"),
    [
        (Reply(503, "application/json", ENGINE_STOPPED), "provider_server", True, "the engine is not running"),
        (Reply(401, "text/html", LOGIN_PAGE), "provider_auth", False, "401 Unauthorized"),
        (Reply(403, "text/html", LOGIN_PAGE), "provider_auth", False, "403 Forbidden"),
        (Reply(429, "application/json", RATE_LIMITED, {"Retry-After": "3"}), "provider_rate_limit", True, "too many requests"),
    ],
    ids=["503-engine-stopped", "401-html", "403-html", "429"],
)
def test_metadata_failures_of_an_accepted_run(tmp_path: Path, fake: FakeLlamaSwap, reply, code, retryable, text) -> None:
    fake.props = reply
    error = failed_run(swap_workspace(tmp_path, fake.url))
    assert error.code == code and error.retryable is retryable
    assert error.details.status == reply.status and error.details.provider == "llamaswap"
    assert text in error.message and "/upstream/m/props" in error.message
    if code == "provider_rate_limit":
        assert error.details.retry_after_ms == 3000
    assert "/upstream/m/props" in fake.paths


def test_unreachable_server_is_a_network_failure(tmp_path: Path, fake: FakeLlamaSwap) -> None:
    url = fake.url
    fake.shutdown()
    fake.server_close()
    error = failed_run(swap_workspace(tmp_path, url))
    assert error.code == "provider_network" and error.retryable


def test_inference_failure_after_metadata(tmp_path: Path, fake: FakeLlamaSwap) -> None:
    fake.chat = Reply(503, "application/json", ENGINE_STOPPED)
    error = failed_run(swap_workspace(tmp_path, fake.url))
    assert error.code == "provider_server" and error.retryable and error.details.status == 503
    assert "the engine is not running" in error.message and "/v1/chat/completions" in error.message


def test_startup_metadata_failure_is_typed(tmp_path: Path, fake: FakeLlamaSwap) -> None:
    ws = swap_workspace(tmp_path, fake.url)
    handle = ws.launch()  # a fresh session does not contact the provider
    session = handle.session_id
    handle.stop()

    fake.props = Reply(503, "application/json", ENGINE_STOPPED)
    ws.extra_args += ["--model", "m"]  # restoring the selection resolves the model at startup
    with pytest.raises(AuraError) as outage:
        ws.launch(resume=session)
    assert outage.value.code == "provider_server" and outage.value.retryable
    assert outage.value.details["status"] == 503 and outage.value.details["provider"] == "llamaswap"
    assert "the engine is not running" in outage.value.message

    fake.props = Reply(401, "text/html", LOGIN_PAGE)
    with pytest.raises(AuraError) as credentials:
        ws.launch(resume=session)
    assert credentials.value.code == "provider_auth" and not credentials.value.retryable
