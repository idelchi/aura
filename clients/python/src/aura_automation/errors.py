"""Errors raised by the client."""

from typing import Any

from .models import ErrorInfo


class AuraError(Exception):
    """A failure reported by Aura (or detected by the client), with its contract code."""

    def __init__(
        self,
        code: str,
        message: str,
        *,
        details: dict[str, Any] | None = None,
        status: int | None = None,
        retryable: bool = False,
    ) -> None:
        super().__init__(f"{code}: {message}")
        self.code = code
        self.message = message
        self.details = details or {}
        self.status = status
        self.retryable = retryable

    @classmethod
    def from_info(cls, info: ErrorInfo, *, status: int | None = None) -> "AuraError":
        return cls(
            info.code,
            info.message,
            details=info.details.model_dump(exclude_none=True),
            status=status,
            retryable=info.retryable,
        )


class StopRequested(Exception):
    """Raised by waiting helpers when the caller's stop predicate turned true.

    The run is not cancelled; ``last_seq`` is the journal position reached.
    """

    def __init__(self, last_seq: int) -> None:
        super().__init__(f"stop requested at journal seq {last_seq}")
        self.last_seq = last_seq
