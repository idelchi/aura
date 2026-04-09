package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/yuin/goldmark"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/part"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/tool/call"
)

// sseEvent is a compact replay record: event name + pre-rendered HTML.
type sseEvent struct {
	Name string
	Data string
}

// Web implements ui.UI with a browser-based interface.
type Web struct {
	events  chan ui.Event
	input   chan ui.UserInput
	actions chan ui.UserAction
	cancel  chan struct{}

	broker     *sseBroker // HTML events for browser clients
	jsonBroker *sseBroker // JSON events for programmatic clients
	md         goldmark.Markdown
	chromaCSS  string

	// Pending blocking responses.
	mu                sync.Mutex
	pendingAsk        chan<- string
	pendingAskOptions []ui.AskOption
	pendingAskMulti   bool
	pendingConfirm    chan<- ui.ConfirmAction

	// Compact replay buffer for SSE reconnects.
	// Contains only message.started and message.finalized events.
	compactBuffer []sseEvent

	// Streaming state.
	status       ui.Status
	hints        ui.DisplayHints
	verbose      bool
	tracker      ui.PartTracker
	currentMsgID string
	partCounter  int

	// HTTP server.
	addr string
}

// New creates a Web UI bound to the given address.
func New(addr string) *Web {
	return &Web{
		events:     make(chan ui.Event, 100),
		input:      make(chan ui.UserInput),
		actions:    make(chan ui.UserAction),
		cancel:     make(chan struct{}),
		broker:     &sseBroker{},
		jsonBroker: &sseBroker{},
		md:         newMarkdownRenderer(),
		chromaCSS:  generateChromaCSS(),
		addr:       addr,
	}
}

func (w *Web) Events() chan<- ui.Event           { return w.events }
func (w *Web) Input() <-chan ui.UserInput        { return w.input }
func (w *Web) Actions() <-chan ui.UserAction     { return w.actions }
func (w *Web) Cancel() <-chan struct{}           { return w.cancel }
func (w *Web) SetHintFunc(_ func(string) string) {}
func (w *Web) SetWorkdir(_ string)               {}

// appendReplay adds an event to the compact replay buffer.
func (w *Web) appendReplay(name, data string) {
	w.mu.Lock()
	w.compactBuffer = append(w.compactBuffer, sseEvent{Name: name, Data: data})
	w.mu.Unlock()
}

// clearMessages empties the browser's message area and the replay buffer.
func (w *Web) clearMessages() {
	w.mu.Lock()
	w.compactBuffer = w.compactBuffer[:0]
	w.mu.Unlock()

	// OOB swap to replace #messages content with empty string.
	w.broker.Broadcast("message.finalized", `<div id="messages" hx-swap-oob="innerHTML"></div>`)
}

// Run starts the HTTP server, opens the browser, and consumes events until context cancellation.
func (w *Web) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	w.registerRoutes(mux)

	server := &http.Server{Addr: w.addr, Handler: mux}

	// Start HTTP server in background.
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			debug.Log("[web] HTTP server error: %v", err)
		}
	}()

	url := "http://" + w.addr
	fmt.Printf("Web UI: %s\n", url)

	if err := openBrowser(url); err != nil {
		fmt.Printf("Open %s in your browser\n", url)
	}

	// Consume events in background goroutine.
	go w.consumeEvents(ctx)

	// Block until context is cancelled.
	<-ctx.Done()

	// Graceful shutdown.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	server.Shutdown(shutdownCtx)
	w.drain()

	return ctx.Err()
}

func (w *Web) registerRoutes(mux *http.ServeMux) {
	// Static files.
	mux.HandleFunc("GET /", w.serveIndex)
	mux.HandleFunc("GET /htmx.min.js", w.serveStatic("static/htmx.min.js", "application/javascript"))
	mux.HandleFunc("GET /htmx-sse.js", w.serveStatic("static/htmx-sse.js", "application/javascript"))
	mux.HandleFunc("GET /chroma.css", w.serveChromaCSS)

	// SSE endpoints.
	mux.HandleFunc("GET /events", w.handleEvents)
	mux.HandleFunc("GET /events/json", w.handleEventsJSON)

	// POST endpoints.
	mux.HandleFunc("POST /send", w.handleSend)
	mux.HandleFunc("POST /cancel", w.handleCancel)
	mux.HandleFunc("POST /command", w.handleCommand)
	mux.HandleFunc("POST /ask", w.handleAsk)
	mux.HandleFunc("POST /confirm", w.handleConfirm)
}

func (w *Web) serveIndex(wr http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(wr, r)

		return
	}

	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(wr, "index.html not found", http.StatusInternalServerError)

		return
	}

	wr.Header().Set("Content-Type", "text/html; charset=utf-8")
	wr.Write(data)
}

func (w *Web) serveStatic(path, contentType string) http.HandlerFunc {
	return func(wr http.ResponseWriter, r *http.Request) {
		data, err := staticFS.ReadFile(path)
		if err != nil {
			http.NotFound(wr, r)

			return
		}

		wr.Header().Set("Content-Type", contentType)
		wr.Write(data)
	}
}

func (w *Web) serveChromaCSS(wr http.ResponseWriter, r *http.Request) {
	wr.Header().Set("Content-Type", "text/css")
	wr.Write([]byte(w.chromaCSS))
}

func (w *Web) handleEvents(wr http.ResponseWriter, r *http.Request) {
	flusher, ok := wr.(http.Flusher)
	if !ok {
		http.Error(wr, "SSE not supported", http.StatusInternalServerError)

		return
	}

	wr.Header().Set("Content-Type", "text/event-stream")
	wr.Header().Set("Cache-Control", "no-cache")
	wr.Header().Set("Connection", "keep-alive")
	wr.Header().Set("X-Accel-Buffering", "no")

	id, ch := w.broker.AddClient()
	defer w.broker.RemoveClient(id)

	// Snapshot status and replay buffer under a single lock.
	w.mu.Lock()
	status := w.status
	hints := w.hints
	replay := make([]sseEvent, len(w.compactBuffer))
	copy(replay, w.compactBuffer)
	w.mu.Unlock()

	// Send current status immediately so new clients don't stay on "Connecting...".
	if status.StatusLine(hints) != "" {
		fmt.Fprint(wr, formatSSE("status", renderStatus(status, hints)))
	}

	for _, ev := range replay {
		fmt.Fprint(wr, formatSSE(ev.Name, ev.Data))
	}

	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			fmt.Fprint(wr, msg)
			flusher.Flush()
		}
	}
}

func (w *Web) handleEventsJSON(wr http.ResponseWriter, r *http.Request) {
	flusher, ok := wr.(http.Flusher)
	if !ok {
		http.Error(wr, "SSE not supported", http.StatusInternalServerError)

		return
	}

	wr.Header().Set("Content-Type", "text/event-stream")
	wr.Header().Set("Cache-Control", "no-cache")
	wr.Header().Set("Connection", "keep-alive")
	wr.Header().Set("X-Accel-Buffering", "no")
	wr.Header().Set("Access-Control-Allow-Origin", "*")

	id, ch := w.jsonBroker.AddClient()
	defer w.jsonBroker.RemoveClient(id)

	// Send current status immediately.
	w.mu.Lock()
	status := w.status
	hints := w.hints
	w.mu.Unlock()

	if status.StatusLine(hints) != "" {
		fmt.Fprint(wr, formatSSE("status", marshalJSON(statusJSON(status))))
	}

	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			fmt.Fprint(wr, msg)
			flusher.Flush()
		}
	}
}

// marshalJSON serializes a value to a JSON string. Panics are impossible
// since all inputs are simple maps/slices/strings.
func marshalJSON(v any) string {
	data, _ := json.Marshal(v)

	return string(data)
}

func statusJSON(s ui.Status) map[string]any {
	return map[string]any{
		"agent":       s.Agent,
		"mode":        s.Mode,
		"provider":    s.Provider,
		"model":       s.Model,
		"think":       s.Think.String(),
		"tokens_used": s.Tokens.Used,
		"tokens_max":  s.Tokens.Max,
		"tokens_pct":  s.Tokens.Percent,
	}
}

func displayHintsJSON(h ui.DisplayHints) map[string]any {
	return map[string]any{
		"verbose": h.Verbose,
		"auto":    h.Auto,
	}
}

// consumeEvents processes UI events and broadcasts HTML fragments via SSE.
func (w *Web) consumeEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.events:
			if !ok {
				return
			}

			w.processEvent(event)
		}
	}
}

func (w *Web) processEvent(event ui.Event) {
	switch e := event.(type) {
	case ui.StatusChanged:
		w.mu.Lock()
		w.status = e.Status

		hints := w.hints
		w.mu.Unlock()
		w.broker.Broadcast("status", renderStatus(e.Status, hints))
		w.jsonBroker.Broadcast("status", marshalJSON(statusJSON(e.Status)))

	case ui.DisplayHintsChanged:
		w.mu.Lock()
		w.hints = e.Hints

		status := w.status
		w.mu.Unlock()

		w.verbose = e.Hints.Verbose
		w.broker.Broadcast("status", renderStatus(status, e.Hints))
		w.jsonBroker.Broadcast("display_hints", marshalJSON(displayHintsJSON(e.Hints)))

	case ui.MessageStarted:
		w.currentMsgID = e.MessageID
		w.tracker.Reset()

		w.partCounter = 0

		html := renderMessageStarted(e.MessageID)
		w.broker.Broadcast("message.started", html)
		w.appendReplay("message.started", html)
		w.jsonBroker.Broadcast("message.started", marshalJSON(map[string]any{
			"id": e.MessageID,
		}))

	case ui.MessageAdded:
		msgJSON := marshalJSON(map[string]any{
			"role":    string(e.Message.Role),
			"content": strings.TrimSpace(e.Message.Content),
		})

		if e.Message.IsBookmark() {
			html := fmt.Sprintf(`<div class="bookmark"><hr><span>%s</span></div>`, strings.TrimSpace(e.Message.Content))
			w.broker.Broadcast("message.added", html)
			w.appendReplay("message.added", html)
		} else if e.Message.IsDisplayOnly() {
			html := fmt.Sprintf(`<div class="message display-only">%s</div>`, strings.TrimSpace(e.Message.Content))
			w.broker.Broadcast("message.added", html)
			w.appendReplay("message.added", html)
		} else if e.Message.Role == roles.User {
			html := renderUserMessage(w.status, strings.TrimSpace(e.Message.Content))
			w.broker.Broadcast("message.added", html)
			w.appendReplay("message.added", html)
		}

		w.jsonBroker.Broadcast("message.added", msgJSON)

	case ui.MessagePartAdded:
		idx := w.partCounter
		w.partCounter++
		w.broker.Broadcast("message.part", renderPartAdded(e.MessageID, idx, e.Part))
		w.tracker.Added(e.Part)
		w.jsonBroker.Broadcast("message.part", marshalJSON(map[string]any{
			"id":   e.MessageID,
			"part": idx,
			"type": string(e.Part.Type),
		}))

	case ui.MessagePartUpdated:
		w.handlePartUpdated(e)

	case ui.MessageFinalized:
		html := renderFinalized(e.Message.ID, e.Message, w.md)
		w.broker.Broadcast("message.finalized", html)
		w.appendReplay("message.finalized", html)
		w.tracker.Reset()

		// Extract content, thinking, and calls from parts
		// (top-level fields may be empty for streamed messages).
		var (
			content, thinking strings.Builder
			calls             []map[string]any
		)

		for _, p := range e.Message.Parts {
			switch p.Type {
			case part.Content:
				content.WriteString(p.Text)
			case part.Thinking:
				thinking.WriteString(p.Text)
			case part.Tool:
				if p.Call != nil {
					calls = append(calls, map[string]any{
						"name":   p.Call.Name,
						"args":   p.Call.Arguments,
						"result": p.Call.DisplayResult(),
						"state":  string(p.Call.State),
					})
				}
			}
		}

		w.jsonBroker.Broadcast("message.finalized", marshalJSON(map[string]any{
			"id":       e.Message.ID,
			"role":     string(e.Message.Role),
			"content":  content.String(),
			"thinking": thinking.String(),
			"calls":    calls,
		}))

	case ui.AssistantDone:
		w.broker.Broadcast("assistant.done", renderAssistantDone())

		errStr := ""

		if e.Error != nil {
			errStr = e.Error.Error()

			w.broker.Broadcast("command.result", renderCommandResult(ui.CommandResult{
				Error: e.Error,
			}))
		}

		w.jsonBroker.Broadcast("assistant.done", marshalJSON(map[string]any{
			"error":     errStr,
			"cancelled": e.Cancelled,
		}))

	case ui.SpinnerMessage:
		w.broker.Broadcast("spinner", renderSpinner(e.Text))
		w.jsonBroker.Broadcast("spinner", marshalJSON(map[string]any{"text": e.Text}))

	case ui.ToolOutputDelta:
		w.broker.Broadcast("tool.output", renderToolOutput(e))
		w.jsonBroker.Broadcast("tool.output", marshalJSON(map[string]any{
			"tool": e.ToolName,
			"line": e.Line,
		}))

	case ui.CommandResult:
		if e.Clear {
			w.clearMessages()
		}

		w.broker.Broadcast("command.result", renderCommandResult(e))

		errStr := ""

		if e.Error != nil {
			errStr = e.Error.Error()
		}

		w.jsonBroker.Broadcast("command.result", marshalJSON(map[string]any{
			"command": e.Command,
			"message": e.Message,
			"error":   errStr,
		}))

	case ui.AskRequired:
		w.mu.Lock()
		w.pendingAsk = e.Response
		w.pendingAskOptions = e.Options
		w.pendingAskMulti = e.MultiSelect
		w.mu.Unlock()
		w.broker.Broadcast("ask", renderAskDialog(e))

		opts := make([]map[string]any, len(e.Options))
		for i, o := range e.Options {
			opts[i] = map[string]any{"label": o.Label, "description": o.Description}
		}

		w.jsonBroker.Broadcast("ask", marshalJSON(map[string]any{
			"question":     e.Question,
			"options":      opts,
			"multi_select": e.MultiSelect,
		}))

	case ui.ToolConfirmRequired:
		w.mu.Lock()
		w.pendingConfirm = e.Response
		w.mu.Unlock()
		w.broker.Broadcast("confirm", renderConfirmDialog(e))
		w.jsonBroker.Broadcast("confirm", marshalJSON(map[string]any{
			"tool":        e.ToolName,
			"description": e.Description,
			"detail":      e.Detail,
			"diff":        e.DiffPreview,
			"pattern":     e.Pattern,
		}))

	case ui.SyntheticInjected:
		w.broker.Broadcast("command.result", renderCommandResult(ui.CommandResult{
			Command: e.Header,
			Message: e.Content,
		}))
		w.jsonBroker.Broadcast("synthetic", marshalJSON(map[string]any{
			"header":  e.Header,
			"content": e.Content,
			"role":    e.Role,
		}))

	case ui.Flush:
		close(e.Done)

	case ui.WaitingForInput:
		// No-op.
	case ui.UserMessagesProcessed:
		// No-op.
	case ui.SessionRestored:
		w.clearMessages()

		for _, msg := range e.Messages {
			switch {
			case msg.IsBookmark():
				html := fmt.Sprintf(`<div class="bookmark"><hr><span>%s</span></div>`, strings.TrimSpace(msg.Content))
				w.broker.Broadcast("message.added", html)
				w.appendReplay("message.added", html)
			case msg.IsDisplayOnly():
				html := fmt.Sprintf(`<div class="message display-only">%s</div>`, strings.TrimSpace(msg.Content))
				w.broker.Broadcast("message.added", html)
				w.appendReplay("message.added", html)
			case msg.IsUser():
				html := renderUserMessage(w.status, strings.TrimSpace(msg.Content))
				w.broker.Broadcast("message.added", html)
				w.appendReplay("message.added", html)
			case msg.IsAssistant():
				startHTML := renderMessageStarted(msg.ID)
				w.broker.Broadcast("message.started", startHTML)
				w.appendReplay("message.started", startHTML)

				finalHTML := renderFinalized(msg.ID, msg, w.md)
				w.broker.Broadcast("message.finalized", finalHTML)
				w.appendReplay("message.finalized", finalHTML)
			}
		}

	case ui.SlashCommandHandled:
		// No-op.
	case ui.PickerOpen:
		w.broker.Broadcast("command.result", renderPickerAsText(e))
	case ui.TodoEditRequested:
		// No-op for web UI.
	}
}

func (w *Web) handlePartUpdated(e ui.MessagePartUpdated) {
	d := w.tracker.Updated(e.Part)

	switch e.Part.Type {
	case part.Content:
		if d.Text != "" {
			w.broker.Broadcast("message.update", renderPartDelta(e.MessageID, e.PartIndex, d.Text))
			w.jsonBroker.Broadcast("message.update", marshalJSON(map[string]any{
				"id": e.MessageID, "part": e.PartIndex, "type": "content", "delta": d.Text,
			}))
		}

	case part.Thinking:
		if d.Text != "" {
			if w.verbose {
				w.broker.Broadcast("message.update", renderThinkingDelta(e.MessageID, e.PartIndex, d.Text))
			}

			w.jsonBroker.Broadcast("message.update", marshalJSON(map[string]any{
				"id": e.MessageID, "part": e.PartIndex, "type": "thinking", "delta": d.Text,
			}))
		}

	case part.Tool:
		if e.Part.Call != nil &&
			(e.Part.Call.State == call.Running || e.Part.Call.State == call.Complete || e.Part.Call.State == call.Error) {
			w.broker.Broadcast("message.update", renderToolUpdate(e.MessageID, e.PartIndex, e.Part))
			w.jsonBroker.Broadcast("message.update", marshalJSON(map[string]any{
				"id": e.MessageID, "part": e.PartIndex, "type": "tool",
				"name": e.Part.Call.Name, "state": string(e.Part.Call.State),
				"args": e.Part.Call.Arguments, "result": e.Part.Call.DisplayResult(),
			}))
		}
	}
}

// drain unblocks any pending response channels after context cancellation.
func (w *Web) drain() {
	ui.DrainEvents(w.events)
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("cmd", "/c", "start", url).Start()
	}

	return fmt.Errorf("unsupported platform %s", runtime.GOOS)
}
