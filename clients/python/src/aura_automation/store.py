"""Offline session store operations through ``aura session … --json``."""

import json
import subprocess
from pathlib import Path

from pydantic import BaseModel, ValidationError

from .aura import Aura
from .errors import AuraError
from .models import (
    ErrorInfo,
    ExportResult,
    ForkResult,
    FreezeResult,
    ImportResult,
    SessionList,
    SessionSummary,
)


class Store:
    """The session store of an Aura write home. No model or server is started."""

    def __init__(self, aura: Aura) -> None:
        self.aura = aura

    def list(self) -> SessionList:
        return self._run(SessionList, "list")

    def show(self, session_id: str) -> SessionSummary:
        return self._run(SessionSummary, "show", session_id)

    def import_file(self, path: Path, *, session_id: str | None = None, freeze: bool = False) -> ImportResult:
        """Store a native session file; repeating the import of the same conversation is a no-op."""
        args = ["import", str(path)]
        if session_id:
            args += ["--id", session_id]
        if freeze:
            args.append("--freeze")
        return self._run(ImportResult, *args)

    def export(self, session_id: str, path: Path) -> ExportResult:
        """Copy the stored bytes to ``path``; an identical existing file is left as is."""
        return self._run(ExportResult, "export", session_id, "--output", str(path))

    def fork(
        self,
        parent_id: str,
        child_id: str,
        *,
        title: str | None = None,
        expect_revision: str | None = None,
    ) -> ForkResult:
        """Store an independent copy; the parent is only read. Idempotent for the same parent revision."""
        args = ["fork", parent_id, "--id", child_id]
        if title:
            args += ["--title", title]
        if expect_revision:
            args += ["--expect-revision", expect_revision]
        return self._run(ForkResult, *args)

    def freeze(self, session_id: str, *, expect_revision: str | None = None) -> FreezeResult:
        """Make a session permanently immutable (still forkable)."""
        args = ["freeze", session_id]
        if expect_revision:
            args += ["--expect-revision", expect_revision]
        return self._run(FreezeResult, *args)

    def _run[T: BaseModel](self, model: type[T], *args: str) -> T:
        completed = subprocess.run(
            self.aura.command("session", *args, "--json"),
            cwd=self.aura.cwd,
            env=dict(self.aura.env),
            stdin=subprocess.DEVNULL,
            capture_output=True,
            text=True,
            check=False,
        )
        if completed.returncode != 0:
            raise _cli_error(completed.stderr)
        try:
            return model.model_validate_json(completed.stdout)
        except ValidationError as error:
            raise AuraError("contract_mismatch", f"unexpected `aura session {args[0]}` output: {error}") from error


def _cli_error(stderr: str) -> AuraError:
    """Decode the JSON error envelope printed on stderr."""
    for line in reversed(stderr.strip().splitlines()):
        try:
            envelope = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(envelope, dict) and "error" in envelope:
            return AuraError.from_info(ErrorInfo.model_validate(envelope["error"]))
    return AuraError("cli_failed", stderr.strip()[-2000:] or "aura session failed without output")
