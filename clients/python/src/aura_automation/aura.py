"""The aura binary and its invocation."""

from collections.abc import Mapping, Sequence
from dataclasses import dataclass, field
from pathlib import Path


@dataclass(frozen=True)
class Aura:
    """How to invoke Aura for one workspace.

    ``args`` are root flags placed before every subcommand (e.g. ``--home``, ``--config``,
    ``--agent``). The store used by ``Store`` and ``Server`` is the session store of the
    write home: the last ``--config`` directory.
    """

    binary: Path
    args: Sequence[str] = ()
    cwd: Path = field(default_factory=Path.cwd)
    env: Mapping[str, str] = field(default_factory=dict)

    def command(self, *subcommand: str) -> list[str]:
        """The full argv for a subcommand."""
        return [str(self.binary), *self.args, *subcommand]
