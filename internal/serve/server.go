package serve

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"uuid"

	"github.com/idelchi/aura/internal/assistant"
	"github.com/idelchi/aura/internal/atomicfile"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/session"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/automation"
	"github.com/idelchi/aura/pkg/llm/roles"
)

// DefaultGrace bounds how long shutdown waits for the active run to settle.
const DefaultGrace = 60 * time.Second

// Options configures a Server.
type Options struct {
	Assistant *assistant.Assistant
	UI        *UI
	Bind      string
	ReadyFile string
	Token     string
	Version   string
	Commit    string
	Grace     time.Duration
}

// pendingInput is an ask or confirm waiting for an answer.
type pendingInput struct {
	kind    automation.InputKind
	ask     chan<- string
	confirm chan<- ui.ConfirmAction
	options []ui.AskOption
	multi   bool
}

// run is the server-side state of one request.
type run struct {
	record       automation.Run
	input        ui.UserInput
	finished     bool
	forwarded    bool   // a command that continued as a turn
	cancelReason string // request or shutdown
	blocked      bool   // needed input under the block policy
	content      string // latest assistant (or command) text
	inputs       map[string]pendingInput
}

// Server serves the /v1 API over one live session.
type Server struct {
	opts      Options
	asst      *assistant.Assistant
	ui        *UI
	journal   *Journal
	live      *broadcaster
	store     *session.Store
	sessionID string
	writeHome string
	mcp       []automation.MCPServer

	mu           sync.Mutex
	runs         map[string]*run
	byRequest    map[string]*run
	active       *run
	idle         chan struct{} // closed while no run is active
	stopping     bool
	broken       error // journal failure: no further runs are accepted
	revision     string
	messages     int
	persistedSeq int64
	status       automation.Status
	selection    automation.SessionMeta
	turn         turn

	http      *http.Server
	listener  net.Listener
	closing   chan struct{} // closed when shutdown starts; ends long-lived streams
	requested chan string   // shutdown requests from the API
	stopOnce  sync.Once
}

// Start makes the session durable, opens and recovers its journal, starts listening
// and writes the ready file. The session lease is already held by the assistant.
func Start(opts Options) (*Server, error) {
	if opts.Grace == 0 {
		opts.Grace = DefaultGrace
	}

	asst := opts.Assistant
	mgr := asst.SessionManager()

	s := &Server{
		opts:      opts,
		asst:      asst,
		ui:        opts.UI,
		live:      newBroadcaster(),
		store:     mgr.Store(),
		writeHome: filepath.Dir(mgr.Store().Dir()),
		runs:      make(map[string]*run),
		byRequest: make(map[string]*run),
		idle:      closedChannel(),
		closing:   make(chan struct{}),
		requested: make(chan string, 1),
	}

	opts.UI.server = s

	// A fresh session is created on disk before anyone can learn its ID.
	if mgr.ActiveRevision() == "" {
		if _, err := mgr.Begin(); err != nil {
			return nil, err
		}

		if err := asst.Persist(ui.PersistStart); err != nil {
			return nil, err
		}
	}

	info, ok := mgr.ActiveInfo()
	if !ok {
		return nil, errors.New("session was not persisted before serving")
	}

	s.sessionID, s.revision, s.messages = info.ID, info.Revision, info.Messages

	journal, events, repaired, err := OpenJournal(filepath.Join(s.store.Dir(), "journal", s.sessionID+".jsonl"), s.sessionID)
	if err != nil {
		return nil, err
	}

	s.journal = journal

	s.recover(events)

	s.mu.Lock()

	s.append(automation.EventServerStarted, "", automation.ServerStarted{
		PID: os.Getpid(), Version: opts.Version, ResumedRevision: info.Revision,
	})

	if repaired {
		s.append(automation.EventNotice, "", automation.Notice{
			Level: "warn", Source: "journal",
			Message: "removed a torn final journal line left by an interrupted write",
		})
	}

	s.markLost(info)

	if s.broken != nil {
		s.mu.Unlock()
		journal.Close()

		return nil, s.broken
	}

	s.mu.Unlock()

	s.mcp = mcpStatus(asst)

	listener, err := net.Listen("tcp", opts.Bind)
	if err != nil {
		journal.Close()

		return nil, fmt.Errorf("listening on %s: %w", opts.Bind, err)
	}

	s.listener = listener
	s.http = &http.Server{Handler: s.routes(), ReadHeaderTimeout: 10 * time.Second}

	go func() {
		if err := s.http.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			debug.Log("[serve] http: %v", err)
		}
	}()

	if err := WriteReady(opts.ReadyFile, s.Ready()); err != nil {
		s.http.Close()
		journal.Close()

		return nil, err
	}

	return s, nil
}

// Ready describes the listening server.
func (s *Server) Ready() automation.Ready {
	return automation.Ready{
		API:       automation.APIVersion,
		URL:       "http://" + s.listener.Addr().String(),
		PID:       os.Getpid(),
		SessionID: s.sessionID,
		Version:   s.opts.Version,
		Commit:    s.opts.Commit,
		Workdir:   s.asst.WorkDir(),
		WriteHome: s.writeHome,
	}
}

// WriteReady atomically writes the ready file (0600).
func WriteReady(path string, ready automation.Ready) error {
	data, err := json.MarshalIndent(ready, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	if err := atomicfile.WriteFile(path, append(data, '\n')); err != nil {
		return fmt.Errorf("writing ready file: %w", err)
	}

	return nil
}

// mcpStatus describes the MCP servers of the session after startup.
func mcpStatus(asst *assistant.Assistant) []automation.MCPServer {
	out := []automation.MCPServer{}

	for _, sess := range asst.MCPSessions() {
		out = append(out, automation.MCPServer{
			Name: sess.Name, Status: "connected", Required: sess.Required, Sandboxed: sess.Sandboxed, Tools: len(sess.MCPTools),
		})
	}

	for _, failed := range asst.MCPFailures() {
		out = append(out, automation.MCPServer{Name: failed.Name, Status: "failed", Required: failed.Required, Error: failed.Error.Error()})
	}

	return out
}

// ShutdownRequested delivers the reason of a shutdown requested through the API.
func (s *Server) ShutdownRequested() <-chan string {
	return s.requested
}

// Shutdown stops accepting runs, cancels the active run and waits up to the grace period
// for it to settle, then stops the HTTP server. A run that does not settle is reported
// lost by the next server on this session.
func (s *Server) Shutdown(reason string) {
	s.stopOnce.Do(func() {
		s.mu.Lock()

		s.stopping = true
		s.append(automation.EventServerStopping, "", automation.ServerStopping{Reason: reason})

		if s.active != nil && !s.active.finished {
			s.requestCancel(s.active, "shutdown")
		}

		idle := s.idle

		s.mu.Unlock()

		select {
		case <-idle:
		case <-time.After(s.opts.Grace):
			debug.Log("[serve] active run did not settle within %s", s.opts.Grace)
		}

		close(s.closing)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := s.http.Shutdown(ctx); err != nil {
			s.http.Close()
		}
	})
}

// Close releases the journal after the engine stopped.
func (s *Server) Close() error {
	return s.journal.Close()
}

// append journals an event; s.mu must be held. A journal failure stops new runs.
func (s *Server) append(typ automation.EventType, runID string, data any) automation.Event {
	event, err := s.journal.Append(typ, runID, data)
	if err != nil {
		debug.Log("[serve] journal: %v", err)

		if s.broken == nil {
			s.broken = err
		}
	}

	return event
}

// closedChannel returns a closed channel.
func closedChannel() chan struct{} {
	ch := make(chan struct{})
	close(ch)

	return ch
}

// recover rebuilds the run registry from journal events.
func (s *Server) recover(events []automation.Event) {
	for _, event := range events {
		switch event.Type {
		case automation.EventRunAccepted, automation.EventRunCancelRequested, automation.EventRunFinished,
			automation.EventRunPersisted, automation.EventRunPersistFailed, automation.EventRunLost:
		default:
			continue
		}

		var data automation.RunData

		if err := json.Unmarshal(event.Data, &data); err != nil {
			continue
		}

		r := s.runs[data.Run.ID]
		if r == nil {
			r = &run{}
			s.runs[data.Run.ID] = r
			s.byRequest[data.Run.RequestID] = r
		}

		r.record = data.Run
		r.finished = data.Run.State != automation.StateAccepted && data.Run.State != automation.StateRunning
	}
}

// markLost reports every run left unsettled by a previous server; s.mu must be held.
func (s *Server) markLost(stored session.Info) {
	for _, r := range s.runs {
		if r.record.State.IsTerminal() {
			continue
		}

		lost := automation.NewError(automation.CodeServerLost, "the server stopped before the run settled")

		if r.finished {
			r.record.PersistError = lost
		} else {
			r.record.Error = lost
		}

		now := time.Now().UTC()
		r.record.State = automation.StateLost
		r.record.SettledAt = &now
		r.record.Persisted = &automation.Persisted{Revision: stored.Revision, Messages: stored.Messages}

		s.append(automation.EventRunLost, r.record.ID, automation.RunData{Run: s.snapshot(r)})
	}
}

// snapshot returns a copy of the run record; s.mu must be held.
func (s *Server) snapshot(r *run) automation.Run {
	record := r.record
	record.Terminal = record.State.IsTerminal()

	calls := make(map[string]int, len(record.Stats.ToolCalls))
	for k, v := range record.Stats.ToolCalls {
		calls[k] = v
	}

	record.Stats.ToolCalls = calls

	if r.content != "" && record.State != automation.StateAccepted && record.State != automation.StateRunning {
		record.Result = &automation.Result{Content: r.content}
	}

	return record
}

// requestDigest identifies a request body for idempotency.
func requestDigest(kind automation.Kind, text string, policy automation.InputPolicy) string {
	data, _ := json.Marshal([]string{string(kind), text, string(policy)})
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:])
}

// Submit accepts a run request. An identical request_id and body returns the existing
// run (replay true) in whatever state it is; a different body is a conflict.
func (s *Server) Submit(req automation.RunRequest) (automation.Run, bool, *automation.Error) {
	if req.RequestID == "" || len(req.RequestID) > 256 {
		return automation.Run{}, false, automation.NewError(automation.CodeBadRequest, "request_id must be 1-256 characters")
	}

	if (req.Prompt == nil) == (req.Command == nil) {
		return automation.Run{}, false, automation.NewError(automation.CodeBadRequest, "exactly one of prompt and command is required")
	}

	policy := req.OnInput
	if policy == "" {
		policy = automation.InputBlock
	}

	if policy != automation.InputBlock && policy != automation.InputWait {
		return automation.Run{}, false, automation.NewError(automation.CodeBadRequest, "on_input must be block or wait")
	}

	kind, input := automation.KindPrompt, ui.UserInput{Kind: ui.InputPrompt}

	if req.Prompt != nil {
		input.Text = *req.Prompt
		if strings.TrimSpace(input.Text) == "" {
			return automation.Run{}, false, automation.NewError(automation.CodeBadRequest, "prompt is empty")
		}
	} else {
		kind, input.Kind, input.Text = automation.KindCommand, ui.InputCommand, strings.TrimSpace(*req.Command)
		if !strings.HasPrefix(input.Text, "/") {
			return automation.Run{}, false, automation.NewError(automation.CodeBadRequest, "command must start with /")
		}
	}

	digest := requestDigest(kind, input.Text, policy)

	s.mu.Lock()

	if existing := s.byRequest[req.RequestID]; existing != nil {
		defer s.mu.Unlock()

		if existing.record.RequestDigest != digest {
			return automation.Run{}, false, automation.NewError(automation.CodeRequestConflict,
				"request_id %q was used for a different request", req.RequestID)
		}

		return s.snapshot(existing), true, nil
	}

	if s.stopping {
		s.mu.Unlock()

		return automation.Run{}, false, automation.NewError(automation.CodeShuttingDown, "the server is shutting down")
	}

	if s.broken != nil {
		s.mu.Unlock()

		return automation.Run{}, false, automation.NewError(automation.CodeInternal, "journal unavailable: %v", s.broken)
	}

	if s.active != nil {
		defer s.mu.Unlock()

		return automation.Run{}, false, automation.NewError(automation.CodeBusy, "run %s has not settled", s.active.record.ID)
	}

	if kind == automation.KindCommand {
		if registry := s.asst.Commands(); registry != nil {
			if cmd, ok := registry.Resolve(input.Text); ok && cmd.Rebinds {
				s.mu.Unlock()

				return automation.Run{}, false, automation.NewError(automation.CodeBadRequest,
					"%s changes the served session; use `aura session` and a new server instead", cmd.Name)
			}
		}
	}

	now := time.Now().UTC()

	r := &run{
		record: automation.Run{
			ID:            "run_" + uuid.NewV7().String(),
			RequestID:     req.RequestID,
			Kind:          kind,
			RequestDigest: digest,
			OnInput:       policy,
			SessionID:     s.sessionID,
			State:         automation.StateAccepted,
			Stats:         automation.Stats{ToolCalls: map[string]int{}},
			AcceptedSeq:   s.journal.LastSeq() + 1,
			AcceptedAt:    now,
		},
		input:  input,
		inputs: make(map[string]pendingInput),
	}

	r.input.ID = r.record.ID

	s.append(automation.EventRunAccepted, r.record.ID, automation.RunData{Run: s.snapshot(r)})

	if s.broken != nil {
		defer s.mu.Unlock()

		return automation.Run{}, false, automation.NewError(automation.CodeInternal, "journal unavailable: %v", s.broken)
	}

	s.runs[r.record.ID] = r
	s.byRequest[req.RequestID] = r
	s.active = r
	s.idle = make(chan struct{})

	accepted := s.snapshot(r)

	s.mu.Unlock()

	// The engine loop is idle while no run is active, so it receives promptly. The
	// lock is released first: the engine may be emitting events that need it. If the
	// loop already stopped, the run stays accepted and the next server reports it lost.
	select {
	case s.ui.input <- r.input:
	case <-s.asst.Done():
	}

	return accepted, false, nil
}

// Run returns a run by ID.
func (s *Server) Run(id string) (automation.Run, *automation.Error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	r := s.runs[id]
	if r == nil {
		return automation.Run{}, automation.NewError(automation.CodeNotFound, "no run %q", id)
	}

	return s.snapshot(r), nil
}

// RunFor returns the run of a request ID.
func (s *Server) RunFor(requestID string) (automation.Run, *automation.Error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	r := s.byRequest[requestID]
	if r == nil {
		return automation.Run{}, automation.NewError(automation.CodeNotFound, "no run for request %q", requestID)
	}

	return s.snapshot(r), nil
}

// Cancel requests cancellation of a run. It is idempotent; a finished run is returned unchanged.
func (s *Server) Cancel(id string) (automation.Run, *automation.Error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	r := s.runs[id]
	if r == nil {
		return automation.Run{}, automation.NewError(automation.CodeNotFound, "no run %q", id)
	}

	if !r.finished && !r.record.CancelRequested {
		s.requestCancel(r, "request")
	}

	return s.snapshot(r), nil
}

// requestCancel records and signals cancellation of an unfinished run; s.mu must be held.
func (s *Server) requestCancel(r *run, reason string) {
	if !r.record.CancelRequested {
		r.record.CancelRequested = true
		r.cancelReason = reason

		s.append(automation.EventRunCancelRequested, r.record.ID, automation.RunData{Run: s.snapshot(r)})
	}

	s.ui.signalCancel()
}

// Answer resolves a pending ask or confirm of the active run.
func (s *Server) Answer(runID, inputID string, answer automation.InputAnswer) *automation.Error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r := s.runs[runID]
	if r == nil {
		return automation.NewError(automation.CodeNotFound, "no run %q", runID)
	}

	pending, ok := r.inputs[inputID]
	if !ok {
		return automation.NewError(automation.CodeNotPending, "input %q of run %s is not pending", inputID, runID)
	}

	var resolved string

	switch pending.kind {
	case automation.InputAsk:
		if answer.Answer == nil || answer.Action != nil {
			return automation.NewError(automation.CodeBadRequest, "an ask is answered with answer")
		}

		resolved = ui.ResolveAskResponse(*answer.Answer, pending.options, pending.multi)
		pending.ask <- resolved
	default:
		if answer.Action == nil || answer.Answer != nil {
			return automation.NewError(automation.CodeBadRequest, "a confirm is answered with action")
		}

		action, ok := map[string]ui.ConfirmAction{
			"allow": ui.ConfirmAllow, "allow_session": ui.ConfirmAllowSession, "deny": ui.ConfirmDeny,
		}[*answer.Action]
		if !ok {
			return automation.NewError(automation.CodeBadRequest, "action must be allow, allow_session or deny")
		}

		resolved = *answer.Action
		pending.confirm <- action
	}

	delete(r.inputs, inputID)

	s.append(automation.EventInputResolved, runID, automation.InputResolved{InputID: inputID, Answer: resolved})

	return nil
}

// Info describes the server.
func (s *Server) Info() automation.ServerInfo {
	s.mu.Lock()
	defer s.mu.Unlock()

	info := automation.ServerInfo{
		API:          automation.APIVersion,
		Version:      s.opts.Version,
		Commit:       s.opts.Commit,
		PID:          os.Getpid(),
		SessionID:    s.sessionID,
		Workdir:      s.asst.WorkDir(),
		WriteHome:    s.writeHome,
		LastSeq:      s.journal.LastSeq(),
		ShuttingDown: s.stopping,
		MCP:          s.mcp,
	}

	if s.active != nil {
		info.ActiveRun = automation.NullableString(s.active.record.ID)
	}

	return info
}

// Session describes the served session as stored, with its live selection and status.
func (s *Server) Session() (automation.SessionInfo, *automation.Error) {
	stored, err := s.store.Info(s.sessionID)
	if err != nil {
		return automation.SessionInfo{}, Classify(err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return automation.SessionInfo{
		SessionSummary: stored,
		PersistedSeq:   s.persistedSeq,
		Meta:           s.selection,
		Status: automation.SessionStatus{
			Iteration: s.status.Iteration, TokensUsed: s.status.TokensUsed, TokensMax: s.status.TokensMax,
		},
	}, nil
}

// Messages returns the stored non-system messages from index after, without thinking unless asked.
func (s *Server) Messages(after int, thinking bool) (automation.Messages, *automation.Error) {
	stored, err := s.store.Load(s.sessionID)
	if err != nil {
		return automation.Messages{}, Classify(err)
	}

	var all []map[string]any

	for _, msg := range stored.Messages {
		if msg.Role == roles.System {
			continue
		}

		data, err := json.Marshal(msg)
		if err != nil {
			return automation.Messages{}, Classify(err)
		}

		var object map[string]any
		if err := json.Unmarshal(data, &object); err != nil {
			return automation.Messages{}, Classify(err)
		}

		if !thinking {
			delete(object, "Thinking")
			delete(object, "thinking_signature")
		}

		all = append(all, object)
	}

	if after < 0 || after > len(all) {
		return automation.Messages{}, automation.NewError(automation.CodeBadRequest, "after must be between 0 and %d", len(all))
	}

	page := all[after:]
	if page == nil {
		page = []map[string]any{}
	}

	return automation.Messages{Messages: page, Total: len(all)}, nil
}
