package serve

import (
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/automation"
	"github.com/idelchi/aura/pkg/llm/part"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/tool/call"
)

// turn mirrors the parts of the assistant message being built, so that each iteration's
// text and tool calls are journaled once, in order, before the calls' execution events.
type turn struct {
	messageID string
	parts     []part.Part
	flushed   int
	started   map[string]bool
	finished  map[string]bool
}

// reset starts tracking a new assistant message.
func (t *turn) reset(messageID string) {
	*t = turn{messageID: messageID, started: map[string]bool{}, finished: map[string]bool{}}
}

// handle translates one engine event into journal entries and run transitions.
func (s *Server) handle(event ui.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch e := event.(type) {
	case ui.StatusChanged:
		s.onStatus(e.Status)
	case ui.UserMessagesProcessed:
		if r := s.activeFor(e.InputIDs...); r != nil && r.record.CancelRequested {
			s.ui.signalCancel()
		}
	case ui.InputAdmitted:
		s.onAdmitted(e)
	case ui.MessageAdded:
		if e.Message.Role == roles.User && !e.Message.IsInternal() {
			s.append(automation.EventMessageUser, s.activeID(), automation.MessageUser{
				MessageID: e.Message.ID, Content: e.Message.Content,
			})
		}
	case ui.MessageStarted:
		s.turn.reset(e.MessageID)
	case ui.MessagePartAdded:
		s.turn.parts = append(s.turn.parts, e.Part)
		s.publishDelta(e.MessageID, e.Part, "")
	case ui.MessagePartUpdated:
		s.onPartUpdated(e)
	case ui.MessageFinalized:
		s.flushTurn()
	case ui.AskRequired:
		s.onAsk(e)
	case ui.ToolConfirmRequired:
		s.onConfirm(e)
	case ui.CommandResult:
		s.onCommandResult(e)
	case ui.SyntheticInjected:
		s.append(automation.EventNotice, s.activeID(), automation.Notice{
			Level: "info", Source: "synthetic", Message: strings.TrimSpace(e.Header + "\n" + e.Content),
		})
	case ui.PickerOpen:
		s.append(automation.EventNotice, s.activeID(), automation.Notice{Level: "info", Source: "command", Message: e.Display()})
	case ui.SlashCommandHandled:
		s.onCommandHandled(e)
	case ui.AssistantDone:
		s.onDone(e)
	case ui.SessionPersisted:
		s.onPersisted(e)
	case ui.SessionPersistFailed:
		s.onPersistFailed(e)
	case ui.SpinnerMessage:
		s.live.publish(transient{Type: automation.EventSpinner, Data: automation.Spinner{Text: e.Text}})
	case ui.ToolOutputDelta:
		s.live.publish(transient{Type: automation.EventToolOutput, Data: automation.ToolOutput{Name: e.ToolName, Line: e.Line}})
	case ui.Flush:
		close(e.Done)
	}
}

// activeID returns the ID of the active run, or empty.
func (s *Server) activeID() string {
	if s.active == nil {
		return ""
	}

	return s.active.record.ID
}

// activeFor returns the active run if ids names it.
func (s *Server) activeFor(ids ...string) *run {
	if s.active != nil && slices.Contains(ids, s.active.record.ID) {
		return s.active
	}

	return nil
}

func (s *Server) onStatus(status ui.Status) {
	next := automation.Status{
		Agent: status.Agent, Mode: status.Mode, Provider: status.Provider, Model: status.Model,
		Iteration: status.Steps.Current, TokensUsed: status.Tokens.Used, TokensMax: status.Tokens.Max,
	}

	s.selection = automation.SessionMeta{
		Agent: status.Agent, Mode: status.Mode, Model: status.Model, Provider: status.Provider,
		Think: status.Think.AsString(), Sandbox: status.Sandbox.Enabled,
	}

	if s.active != nil && next.Iteration > s.active.record.Stats.Iterations {
		s.active.record.Stats.Iterations = next.Iteration
	}

	if next == s.status {
		return
	}

	s.status = next
	s.append(automation.EventStatus, s.activeID(), next)
}

func (s *Server) onAdmitted(e ui.InputAdmitted) {
	r := s.activeFor(e.InputID)
	if r == nil {
		return
	}

	now := time.Now().UTC()
	r.record.State = automation.StateRunning
	r.record.StartedAt = &now

	s.append(automation.EventRunStarted, r.record.ID, automation.RunStarted{RunID: r.record.ID, MessageID: e.MessageID})

	if r.record.CancelRequested {
		s.ui.signalCancel()
	}
}

// publishDelta streams the new text of a content or thinking part (transient).
func (s *Server) publishDelta(messageID string, p part.Part, previous string) {
	delta := strings.TrimPrefix(p.Text, previous)
	if delta == "" {
		return
	}

	switch p.Type {
	case part.Content:
		s.live.publish(transient{Type: automation.EventDeltaContent, Data: automation.Delta{MessageID: messageID, Text: delta}})
	case part.Thinking:
		s.live.publish(transient{Type: automation.EventDeltaThinking, Data: automation.Delta{MessageID: messageID, Text: delta}})
	}
}

func (s *Server) onPartUpdated(e ui.MessagePartUpdated) {
	if e.PartIndex < 0 || e.PartIndex >= len(s.turn.parts) {
		return
	}

	previous := s.turn.parts[e.PartIndex].Text
	s.turn.parts[e.PartIndex] = e.Part
	s.publishDelta(e.MessageID, e.Part, previous)

	c := e.Part.Call
	if e.Part.Type != part.Tool || c == nil {
		return
	}

	switch c.State {
	case call.Running:
		if s.turn.started[c.ID] {
			return
		}

		s.flushTurn()
		s.turn.started[c.ID] = true
		s.append(automation.EventToolStarted, s.activeID(), automation.ToolStarted{CallID: c.ID, Name: c.Name, Args: c.Arguments})
	case call.Complete, call.Error:
		if s.turn.finished[c.ID] {
			return
		}

		s.flushTurn()
		s.turn.finished[c.ID] = true

		finished := automation.ToolFinished{CallID: c.ID, Name: c.Name, Status: automation.ToolOK, Result: c.FullOutput}
		if c.State == call.Error {
			finished.Status, finished.Result = automation.ToolError, ""
			if c.Error != nil {
				finished.Error = c.Error.Error()
			}
		}

		if r := s.active; r != nil {
			r.record.Stats.ToolCalls[c.Name]++
			if c.State == call.Error {
				r.record.Stats.ToolErrors++
			}
		}

		s.append(automation.EventToolFinished, s.activeID(), finished)
	}
}

// flushTurn journals the text and tool calls added since the last flush as one
// message.assistant entry. Thinking is never journaled.
func (s *Server) flushTurn() {
	var (
		text  strings.Builder
		calls []automation.ToolCall
	)

	for _, p := range s.turn.parts[s.turn.flushed:] {
		switch {
		case p.Type == part.Content:
			text.WriteString(p.Text)
		case p.Type == part.Tool && p.Call != nil:
			calls = append(calls, automation.ToolCall{CallID: p.Call.ID, Name: p.Call.Name, Args: p.Call.Arguments})
		}
	}

	s.turn.flushed = len(s.turn.parts)

	content := strings.TrimSpace(text.String())
	if content == "" && len(calls) == 0 {
		return
	}

	if calls == nil {
		calls = []automation.ToolCall{}
	}

	if r := s.active; r != nil && content != "" {
		r.content = content
	}

	s.append(automation.EventMessageAssistant, s.activeID(), automation.MessageAssistant{
		MessageID: s.turn.messageID, Content: content, ToolCalls: calls,
	})
}

func (s *Server) onAsk(e ui.AskRequired) {
	r := s.active
	if r == nil || r.finished {
		e.Response <- ""

		return
	}

	if r.record.OnInput == automation.InputBlock {
		r.blocked = true
		s.requestCancel(r, "request")
		e.Response <- ""

		return
	}

	options := make([]automation.Option, len(e.Options))
	for i, o := range e.Options {
		options[i] = automation.Option{Label: o.Label, Description: o.Description}
	}

	id := "input_" + uuid.NewV7().String()
	r.inputs[id] = pendingInput{kind: automation.InputAsk, ask: e.Response, options: e.Options, multi: e.MultiSelect}

	s.append(automation.EventInputRequired, r.record.ID, automation.InputRequired{
		InputID: id, Kind: automation.InputAsk, Question: e.Question, Options: options, MultiSelect: e.MultiSelect,
	})
}

func (s *Server) onConfirm(e ui.ToolConfirmRequired) {
	r := s.active
	if r == nil || r.finished {
		e.Response <- ui.ConfirmDeny

		return
	}

	if r.record.OnInput == automation.InputBlock {
		r.blocked = true
		s.requestCancel(r, "request")
		e.Response <- ui.ConfirmDeny

		return
	}

	id := "input_" + uuid.NewV7().String()
	r.inputs[id] = pendingInput{kind: automation.InputConfirm, confirm: e.Response}

	s.append(automation.EventInputRequired, r.record.ID, automation.InputRequired{
		InputID: id, Kind: automation.InputConfirm, Options: []automation.Option{},
		Tool: e.ToolName, Detail: e.Detail, Pattern: e.Pattern,
	})
}

func (s *Server) onCommandResult(e ui.CommandResult) {
	message := e.Message

	errText := ""
	if e.Error != nil {
		errText = e.Error.Error()
	}

	if message == "" && errText == "" {
		return // command echo
	}

	if r := s.active; r != nil && r.record.Kind == automation.KindCommand && !r.forwarded && !r.finished {
		if message != "" {
			r.content = message
		}

		s.append(automation.EventCommandResult, r.record.ID, automation.CommandResult{Command: e.Command, Message: message, Error: errText})

		return
	}

	level := "info"

	switch {
	case e.Error != nil || e.Level == ui.LevelError:
		level = "error"
	case e.Level == ui.LevelWarn:
		level = "warn"
	}

	if message == "" {
		message = errText
	}

	source := "engine"
	if e.Command != "" {
		source = strings.TrimPrefix(e.Command, "/")
	}

	s.append(automation.EventNotice, s.activeID(), automation.Notice{Level: level, Source: source, Message: message})
}

func (s *Server) onCommandHandled(e ui.SlashCommandHandled) {
	r := s.activeFor(e.InputID)
	if r == nil {
		return
	}

	if e.Forwarded {
		r.forwarded = true

		return
	}

	var failure *automation.Error

	if e.Error != nil {
		failure = Classify(e.Error)
		if failure.Code == automation.CodeInternal {
			failure.Code = automation.CodeCommandFailed
		}
	}

	s.finish(r, failure)
}

func (s *Server) onDone(e ui.AssistantDone) {
	r := s.activeFor(e.InputIDs...)
	if r == nil || r.finished {
		return
	}

	s.flushTurn()

	var failure *automation.Error

	switch {
	case r.blocked:
		failure = automation.NewError(automation.CodeInputRequired,
			"the run needed an answer or approval, but on_input is block")
	case e.Error != nil:
		failure = Classify(e.Error)
		if failure.Code == automation.CodeCancelled {
			failure.Details.Reason = r.cancelReason
			if failure.Details.Reason == "" {
				failure.Details.Reason = "shutdown"
			}
		}
	case r.record.StartedAt == nil:
		failure = automation.NewError(automation.CodeInputRejected, "the input was empty after directive expansion")
	}

	s.finish(r, failure)
}

// finish records the outcome of a run; persistence settles it later.
func (s *Server) finish(r *run, failure *automation.Error) {
	now := time.Now().UTC()

	r.finished = true
	r.record.State = automation.StateFinished
	r.record.FinishedAt = &now
	r.record.Outcome = automation.OutcomeCompleted
	r.record.Error = failure

	if failure != nil {
		r.record.Outcome = failure.Code.Outcome()
	}

	// Answers can no longer be delivered.
	r.inputs = map[string]pendingInput{}

	s.append(automation.EventRunFinished, r.record.ID, automation.RunData{Run: s.snapshot(r)})
}

func (s *Server) onPersisted(e ui.SessionPersisted) {
	r := s.activeFor(e.InputIDs...)

	event := s.append(automation.EventSessionPersisted, idOf(r), automation.SessionPersisted{
		Revision: e.Revision, Messages: e.Messages, Reason: string(e.Reason), Changed: e.Changed,
	})

	if e.Revision != "" {
		s.revision, s.messages, s.persistedSeq = e.Revision, e.Messages, event.Seq
	}

	if r == nil {
		return
	}

	r.record.Persisted = &automation.Persisted{Revision: e.Revision, Messages: e.Messages, Seq: event.Seq}

	if r.finished && (e.Reason == ui.PersistRun || e.Reason == ui.PersistCommand) {
		s.settle(r, automation.StatePersisted, nil)
	}
}

func (s *Server) onPersistFailed(e ui.SessionPersistFailed) {
	r := s.activeFor(e.InputIDs...)

	failure := Classify(e.Err)
	failure.Code = automation.CodePersistFailed

	s.append(automation.EventSessionPersistFailed, idOf(r), automation.SessionPersistFailed{Error: *failure, Reason: string(e.Reason)})

	if r != nil && r.finished && (e.Reason == ui.PersistRun || e.Reason == ui.PersistCommand) {
		s.settle(r, automation.StatePersistFailed, failure)
	}
}

// settle makes a finished run terminal and frees the server for the next run.
func (s *Server) settle(r *run, state automation.State, persistErr *automation.Error) {
	now := time.Now().UTC()

	r.record.State = state
	r.record.SettledAt = &now
	r.record.PersistError = persistErr

	typ := automation.EventRunPersisted
	if state == automation.StatePersistFailed {
		typ = automation.EventRunPersistFailed
	}

	s.append(typ, r.record.ID, automation.RunData{Run: s.snapshot(r)})

	s.active = nil
	s.ui.clearCancel()
	close(s.idle)
}

// idOf returns the run's ID, or empty for nil.
func idOf(r *run) string {
	if r == nil {
		return ""
	}

	return r.record.ID
}
