package automation

import "fmt"

// Code classifies a request failure or a run outcome.
type Code string

// Request and store failures.
const (
	CodeBadRequest       Code = "bad_request"
	CodeUnauthorized     Code = "unauthorized"
	CodeNotFound         Code = "not_found"
	CodeBusy             Code = "busy"
	CodeRequestConflict  Code = "request_conflict"
	CodeCursorOutOfRange Code = "cursor_out_of_range"
	CodeNotPending       Code = "not_pending"
	CodeShuttingDown     Code = "shutting_down"
	CodeExists           Code = "exists"
	CodeRevisionMismatch Code = "revision_mismatch"
	CodeSessionLocked    Code = "session_locked"
	CodeSessionFrozen    Code = "session_frozen"
	CodeInvalidID        Code = "invalid_id"
	CodeInvalidSession   Code = "invalid_session"
	CodeServerLost       Code = "server_lost"
)

// Server startup failures, reported in the ready file.
const (
	CodeResumeFailed   Code = "resume_failed"
	CodeMCPUnavailable Code = "mcp_unavailable"
	CodeConfigInvalid  Code = "config_invalid"
	CodeStartupFailed  Code = "startup_failed"
)

// Run failures.
const (
	CodeCancelled                Code = "cancelled"
	CodeInputRejected            Code = "input_rejected"
	CodeGuardrailBlocked         Code = "guardrail_blocked"
	CodeInputRequired            Code = "input_required"
	CodePolicyStop               Code = "policy_stop"
	CodeProviderNetwork          Code = "provider_network"
	CodeProviderServer           Code = "provider_server"
	CodeProviderRateLimit        Code = "provider_rate_limit"
	CodeProviderAuth             Code = "provider_auth"
	CodeProviderCredit           Code = "provider_credit"
	CodeProviderModelUnavailable Code = "provider_model_unavailable"
	CodeContentFilter            Code = "content_filter"
	CodeContextExhausted         Code = "context_exhausted"
	CodeCompactionFailed         Code = "compaction_failed"
	CodeMaxSteps                 Code = "max_steps"
	CodeTokenBudget              Code = "token_budget"
	CodeCommandFailed            Code = "command_failed"
	CodePersistFailed            Code = "persist_failed"
	CodeInternal                 Code = "internal"
)

// Retryable reports whether the same request may succeed after waiting: only transient provider failures.
func (c Code) Retryable() bool {
	switch c {
	case CodeProviderNetwork, CodeProviderServer, CodeProviderRateLimit:
		return true
	default:
		return false
	}
}

// Outcome returns the run outcome a run error with this code produces.
func (c Code) Outcome() Outcome {
	switch c {
	case CodeCancelled:
		return OutcomeCancelled
	case CodeInputRejected, CodeGuardrailBlocked:
		return OutcomeRejected
	case CodeInputRequired, CodePolicyStop:
		return OutcomeBlocked
	default:
		return OutcomeFailed
	}
}

// Details carries optional structured context of an Error.
type Details struct {
	Status       int    `json:"status,omitempty"`         // provider HTTP status, when known
	Provider     string `json:"provider,omitempty"`       // provider name, when known
	RetryAfterMS int64  `json:"retry_after_ms,omitempty"` // provider retry hint
	Reason       string `json:"reason,omitempty"`         // cancellation reason: request or shutdown
	LastSeq      *int64 `json:"last_seq,omitempty"`       // journal end for cursor_out_of_range
}

// Error is the wire form of every failure.
type Error struct {
	Code      Code    `json:"code"`
	Message   string  `json:"message"`
	Retryable bool    `json:"retryable"`
	Details   Details `json:"details"`
}

// NewError builds an Error with the code's retryability.
func NewError(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), Retryable: code.Retryable()}
}

// Error implements error.
func (e *Error) Error() string {
	return string(e.Code) + ": " + e.Message
}

// Envelope wraps an Error in responses and CLI error output.
type Envelope struct {
	Error *Error `json:"error"`
}
