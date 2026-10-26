package automation

import (
	"encoding/json"
	"time"
)

// APIVersion is the version of the /v1 contract.
const APIVersion = 1

// Kind is the kind of run request.
type Kind string

const (
	KindPrompt  Kind = "prompt"
	KindCommand Kind = "command"
)

// InputPolicy selects what happens when a run needs an answer (ask) or approval (confirm).
type InputPolicy string

const (
	// InputBlock ends the run as blocked (input_required) instead of waiting.
	InputBlock InputPolicy = "block"
	// InputWait publishes input.required and waits for an answer.
	InputWait InputPolicy = "wait"
)

// State is the lifecycle position of a run.
type State string

const (
	StateAccepted      State = "accepted"
	StateRunning       State = "running"
	StateFinished      State = "finished"
	StatePersisted     State = "persisted"
	StatePersistFailed State = "persist_failed"
	StateLost          State = "lost"
)

// IsTerminal reports whether no further transition can happen.
func (s State) IsTerminal() bool {
	return s == StatePersisted || s == StatePersistFailed || s == StateLost
}

// Outcome is how a finished run ended. The empty outcome is encoded as null.
type Outcome string

const (
	OutcomeCompleted Outcome = "completed"
	OutcomeFailed    Outcome = "failed"
	OutcomeCancelled Outcome = "cancelled"
	OutcomeRejected  Outcome = "rejected"
	OutcomeBlocked   Outcome = "blocked"
)

// MarshalJSON encodes the empty outcome as null.
func (o Outcome) MarshalJSON() ([]byte, error) {
	if o == "" {
		return []byte("null"), nil
	}

	return json.Marshal(string(o))
}

// UnmarshalJSON decodes null as the empty outcome.
func (o *Outcome) UnmarshalJSON(data []byte) error {
	var s *string

	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	*o = ""
	if s != nil {
		*o = Outcome(*s)
	}

	return nil
}

// RunRequest is the body of POST /v1/runs. Exactly one of Prompt and Command is set.
type RunRequest struct {
	RequestID string      `json:"request_id"`
	Prompt    *string     `json:"prompt,omitempty"`
	Command   *string     `json:"command,omitempty"`
	OnInput   InputPolicy `json:"on_input,omitempty"`
}

// Persisted locates the saved session state a run produced.
type Persisted struct {
	Revision string `json:"revision"`
	Messages int    `json:"messages"`
	Seq      int64  `json:"seq"`
}

// Stats counts a run's work.
type Stats struct {
	Iterations int            `json:"iterations"`
	ToolCalls  map[string]int `json:"tool_calls"`
	ToolErrors int            `json:"tool_errors"`
}

// Result is a run's final output.
type Result struct {
	Content string `json:"content"`
}

// Run is the record of one request.
type Run struct {
	ID              string      `json:"id"`
	RequestID       string      `json:"request_id"`
	Kind            Kind        `json:"kind"`
	RequestDigest   string      `json:"request_digest"` // sha256 of the canonical request body
	OnInput         InputPolicy `json:"on_input"`
	SessionID       string      `json:"session_id"`
	State           State       `json:"state"`
	Terminal        bool        `json:"terminal"`
	Outcome         Outcome     `json:"outcome"`
	Error           *Error      `json:"error"`
	PersistError    *Error      `json:"persist_error"`
	CancelRequested bool        `json:"cancel_requested"`
	Result          *Result     `json:"result"`
	Persisted       *Persisted  `json:"persisted"`
	Stats           Stats       `json:"stats"`
	AcceptedSeq     int64       `json:"accepted_seq"`
	AcceptedAt      time.Time   `json:"accepted_at"`
	StartedAt       *time.Time  `json:"started_at"`
	FinishedAt      *time.Time  `json:"finished_at"`
	SettledAt       *time.Time  `json:"settled_at"`
}
