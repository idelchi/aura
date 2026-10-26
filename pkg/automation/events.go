package automation

import (
	"encoding/json"
	"time"
)

// EventType names a journal event.
type EventType string

// Durable event types. Every one is journaled with a sequence number and never contains thinking.
const (
	EventServerStarted        EventType = "server.started"
	EventServerStopping       EventType = "server.stopping"
	EventRunAccepted          EventType = "run.accepted"
	EventRunCancelRequested   EventType = "run.cancel_requested"
	EventRunStarted           EventType = "run.started"
	EventMessageUser          EventType = "message.user"
	EventMessageAssistant     EventType = "message.assistant"
	EventToolStarted          EventType = "tool.started"
	EventToolFinished         EventType = "tool.finished"
	EventNotice               EventType = "notice"
	EventStatus               EventType = "status"
	EventInputRequired        EventType = "input.required"
	EventInputResolved        EventType = "input.resolved"
	EventCommandResult        EventType = "command.result"
	EventRunFinished          EventType = "run.finished"
	EventSessionPersisted     EventType = "session.persisted"
	EventSessionPersistFailed EventType = "session.persist_failed"
	EventRunPersisted         EventType = "run.persisted"
	EventRunPersistFailed     EventType = "run.persist_failed"
	EventRunLost              EventType = "run.lost"
)

// EventHeartbeat is sent on an idle SSE stream (data {}); it is neither journaled nor transient data.
const EventHeartbeat EventType = "heartbeat"

// Transient event types: streamed only with ?transient=true, unsequenced and droppable.
const (
	EventDeltaContent  EventType = "delta.content"
	EventDeltaThinking EventType = "delta.thinking"
	EventToolOutput    EventType = "tool.output"
	EventSpinner       EventType = "spinner"
)

// IsLifecycle reports whether the event marks a lifecycle transition, which is fsynced when journaled.
func (t EventType) IsLifecycle() bool {
	switch t {
	case EventServerStarted, EventServerStopping, EventRunAccepted, EventRunCancelRequested, EventRunStarted, EventRunFinished,
		EventSessionPersisted, EventSessionPersistFailed, EventRunPersisted, EventRunPersistFailed,
		EventRunLost, EventInputRequired, EventInputResolved:
		return true
	default:
		return false
	}
}

// Event is one journal entry. RunID is empty (null) for events outside a run.
type Event struct {
	Seq       int64           `json:"seq"`
	Time      time.Time       `json:"time"`
	Type      EventType       `json:"type"`
	SessionID string          `json:"session_id"`
	RunID     NullableString  `json:"run_id"`
	Data      json.RawMessage `json:"data"`
}

// NullableString encodes the empty string as null.
type NullableString string

// MarshalJSON encodes "" as null.
func (s NullableString) MarshalJSON() ([]byte, error) {
	if s == "" {
		return []byte("null"), nil
	}

	return json.Marshal(string(s))
}

// UnmarshalJSON decodes null as "".
func (s *NullableString) UnmarshalJSON(data []byte) error {
	var v *string

	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}

	*s = ""
	if v != nil {
		*s = NullableString(*v)
	}

	return nil
}

// Journal is the response of GET /v1/journal.
type Journal struct {
	Events  []Event `json:"events"`
	LastSeq int64   `json:"last_seq"`
}

// ServerStarted is the data of server.started.
type ServerStarted struct {
	PID             int    `json:"pid"`
	Version         string `json:"version"`
	ResumedRevision string `json:"resumed_revision"`
}

// ServerStopping is the data of server.stopping.
type ServerStopping struct {
	Reason string `json:"reason"`
}

// RunData is the data of run.accepted, run.cancel_requested, run.finished, run.persisted,
// run.persist_failed and run.lost: the run record after the transition.
type RunData struct {
	Run Run `json:"run"`
}

// RunStarted is the data of run.started: the input entered the conversation.
type RunStarted struct {
	RunID     string `json:"run_id"`
	MessageID string `json:"message_id"`
}

// MessageUser is the data of message.user.
type MessageUser struct {
	MessageID string `json:"message_id"`
	Content   string `json:"content"`
}

// ToolCall is an assistant tool call as journaled.
type ToolCall struct {
	CallID string         `json:"call_id"`
	Name   string         `json:"name"`
	Args   map[string]any `json:"args"`
}

// MessageAssistant is the data of message.assistant (a finalized assistant message, without thinking).
type MessageAssistant struct {
	MessageID string     `json:"message_id"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls"`
}

// ToolStarted is the data of tool.started.
type ToolStarted struct {
	CallID string         `json:"call_id"`
	Name   string         `json:"name"`
	Args   map[string]any `json:"args"`
}

// ToolStatus is the result status of a tool call.
type ToolStatus string

const (
	ToolOK    ToolStatus = "ok"
	ToolError ToolStatus = "error"
)

// ToolFinished is the data of tool.finished. Result is what entered the conversation.
type ToolFinished struct {
	CallID string     `json:"call_id"`
	Name   string     `json:"name"`
	Status ToolStatus `json:"status"`
	Result string     `json:"result"`
	Error  string     `json:"error"`
}

// Notice is the data of notice.
type Notice struct {
	Level   string `json:"level"`
	Source  string `json:"source"`
	Message string `json:"message"`
}

// Status is the data of status (journaled only when it changes).
type Status struct {
	Agent      string `json:"agent"`
	Mode       string `json:"mode"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Iteration  int    `json:"iteration"`
	TokensUsed int    `json:"tokens_used"`
	TokensMax  int    `json:"tokens_max"`
}

// InputKind distinguishes questions from tool approvals.
type InputKind string

const (
	InputAsk     InputKind = "ask"
	InputConfirm InputKind = "confirm"
)

// Option is a selectable answer of an ask.
type Option struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// InputRequired is the data of input.required.
type InputRequired struct {
	InputID     string    `json:"input_id"`
	Kind        InputKind `json:"kind"`
	Question    string    `json:"question"`
	Options     []Option  `json:"options"`
	MultiSelect bool      `json:"multi_select"`
	Tool        string    `json:"tool"`
	Detail      string    `json:"detail"`
	Pattern     string    `json:"pattern"`
}

// InputResolved is the data of input.resolved.
type InputResolved struct {
	InputID string `json:"input_id"`
	Answer  string `json:"answer"`
}

// InputAnswer is the body of POST /v1/runs/{id}/inputs/{input_id}.
type InputAnswer struct {
	Answer *string `json:"answer,omitempty"`
	Action *string `json:"action,omitempty"`
}

// CommandResult is the data of command.result.
type CommandResult struct {
	Command string `json:"command"`
	Message string `json:"message"`
	Error   string `json:"error"`
}

// SessionPersisted is the data of session.persisted.
type SessionPersisted struct {
	Revision string `json:"revision"`
	Messages int    `json:"messages"`
	Reason   string `json:"reason"`
	Changed  bool   `json:"changed"`
}

// SessionPersistFailed is the data of session.persist_failed.
type SessionPersistFailed struct {
	Error  Error  `json:"error"`
	Reason string `json:"reason"`
}

// Delta is the data of the transient delta.content and delta.thinking events.
type Delta struct {
	MessageID string `json:"message_id"`
	Text      string `json:"text"`
}

// ToolOutput is the data of the transient tool.output event.
type ToolOutput struct {
	Name string `json:"name"`
	Line string `json:"line"`
}

// Spinner is the data of the transient spinner event.
type Spinner struct {
	Text string `json:"text"`
}
