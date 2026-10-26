"""Launching and attaching to ``aura serve``."""

import os
import signal
import subprocess
import time
from collections.abc import Callable
from pathlib import Path

from pydantic import ValidationError

from .aura import Aura
from .client import Client
from .errors import AuraError, StopRequested
from .models import API_VERSION, Ready


class ServerHandle:
    """A running server: its address, process and session."""

    def __init__(self, ready: Ready, token: str, process: subprocess.Popen[bytes] | None) -> None:
        assert ready.url and ready.pid and ready.session_id
        self.ready = ready
        self.url = ready.url
        self.pid = ready.pid
        self.session_id = ready.session_id
        self.process = process
        self._token = token

    def client(self, *, timeout: float = 30.0) -> Client:
        return Client(self.url, self._token, timeout=timeout)

    def alive(self) -> bool:
        if self.process is not None:
            return self.process.poll() is None
        return _pid_alive(self.pid)

    def stop(self, *, grace: float = 75.0) -> int | None:
        """Graceful shutdown (settles the active run), then SIGTERM and SIGKILL of the process group.

        Returns the exit status for launched servers, None for attached ones.
        """
        if self.alive():
            try:
                with self.client() as client:
                    client.shutdown()
            except AuraError:
                pass
            if not self._wait_exit(grace):
                _signal_group(self.pid, signal.SIGTERM)
                if not self._wait_exit(10):
                    _signal_group(self.pid, signal.SIGKILL)
                    self._wait_exit(10)
        return self.process.wait() if self.process is not None else None

    def kill(self) -> None:
        """Kill the server's process group immediately (tests and emergency recovery)."""
        _signal_group(self.pid, signal.SIGKILL)
        self._wait_exit(10)

    def _wait_exit(self, timeout: float) -> bool:
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            if not self.alive():
                return True
            time.sleep(0.1)
        return not self.alive()


class Server:
    """Starts or attaches to one ``aura serve`` process (one live session)."""

    @staticmethod
    def launch(
        aura: Aura,
        *,
        ready_file: Path,
        token: str,
        resume: str | None = None,
        bind: str = "127.0.0.1:0",
        log: Path | None = None,
        stop: Callable[[], bool] | None = None,
    ) -> ServerHandle:
        """Start a server in its own process group and wait for its ready file.

        There is no startup timeout (MCP servers may take long to prepare); ``stop`` aborts
        the wait, killing the process. A startup failure raises AuraError with the ready
        file's code, or ``startup_failed`` with the log tail.
        """
        if len(token) < 32:
            raise ValueError("token must hold at least 32 characters")
        ready_file.unlink(missing_ok=True)
        args = ["--resume", resume] if resume else []
        argv = aura.command(*args, "serve", "--bind", bind, "--ready-file", str(ready_file))
        env = {**aura.env, "AURA_SERVE_TOKEN": token}
        output = log.open("ab") if log else subprocess.DEVNULL
        try:
            process = subprocess.Popen(
                argv,
                cwd=aura.cwd,
                env=env,
                stdin=subprocess.DEVNULL,
                stdout=output,
                stderr=subprocess.STDOUT,
                start_new_session=True,
            )
        finally:
            if log:
                output.close()
        try:
            return _await_ready(process, ready_file, token, log, stop)
        except BaseException:
            # Never leave a half-started server behind, whatever went wrong.
            if process.poll() is None:
                _signal_group(process.pid, signal.SIGKILL)
            process.wait()
            raise

    @staticmethod
    def attach(ready_file: Path, token: str) -> ServerHandle:
        """Attach to a running server (e.g. after the controller restarted)."""
        ready = _read_ready(ready_file)
        if ready is None or ready.error is not None or not ready.pid or not _pid_alive(ready.pid):
            raise AuraError("server_unavailable", f"no running server described by {ready_file}")
        handle = ServerHandle(ready, token, None)
        _verify(handle)
        return handle


def _await_ready(
    process: subprocess.Popen[bytes],
    ready_file: Path,
    token: str,
    log: Path | None,
    stop: Callable[[], bool] | None,
) -> ServerHandle:
    """Wait for the ready file of a launched server and verify it."""
    while True:
        ready = _read_ready(ready_file)
        if ready is not None:
            if ready.error is not None:
                _wait_process(process, 10)
                raise AuraError.from_info(ready.error)
            handle = ServerHandle(ready, token, process)
            _verify(handle)
            return handle
        if process.poll() is not None:
            raise AuraError("startup_failed", _tail(log) or f"aura serve exited with status {process.returncode}")
        if stop and stop():
            raise StopRequested(0)
        time.sleep(0.05)


def _verify(handle: ServerHandle) -> None:
    """Check that the address serves the expected contract version, process and session."""
    with handle.client() as client:
        info = client.server()
    if info.api != API_VERSION:
        raise AuraError("contract_mismatch", f"{handle.url} serves api {info.api}, this client speaks {API_VERSION}")
    if info.pid != handle.pid or info.session_id != handle.session_id:
        raise AuraError(
            "server_unavailable",
            f"{handle.url} is pid {info.pid} serving {info.session_id}; expected pid {handle.pid} serving "
            f"{handle.session_id}",
        )


def _read_ready(path: Path) -> Ready | None:
    try:
        return Ready.model_validate_json(path.read_bytes())
    except FileNotFoundError:
        return None
    except ValidationError as error:
        raise AuraError("contract_mismatch", f"unexpected ready file {path}: {error}") from error


def _pid_alive(pid: int) -> bool:
    """Whether pid is a live process; a zombie (exited, not yet reaped) is not."""
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    try:
        stat = Path(f"/proc/{pid}/stat").read_text()
    except FileNotFoundError:
        return False
    except OSError:
        return True
    # The state follows the parenthesised command name, which may itself contain spaces.
    return stat.rsplit(")", 1)[1].split()[0] not in {"Z", "X"}


def _signal_group(pid: int, sig: signal.Signals) -> None:
    try:
        os.killpg(pid, sig)
    except ProcessLookupError:
        pass


def _wait_process(process: subprocess.Popen[bytes], timeout: float) -> None:
    try:
        process.wait(timeout=timeout)
    except subprocess.TimeoutExpired:
        _signal_group(process.pid, signal.SIGKILL)
        process.wait()


def _tail(log: Path | None) -> str:
    if log is None or not log.exists():
        return ""
    return log.read_text(errors="replace")[-4000:].strip()
