package serve

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/idelchi/aura/pkg/automation"
)

// defaultHeartbeat is the interval of heartbeat events on an idle stream.
const defaultHeartbeat = 15 * time.Second

// routes builds the authenticated /v1 handler.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /v1/server", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, s.Info())
	})
	mux.HandleFunc("GET /v1/session", func(w http.ResponseWriter, _ *http.Request) {
		info, e := s.Session()
		respond(w, http.StatusOK, info, e)
	})
	mux.HandleFunc("GET /v1/session/messages", s.handleMessages)
	mux.HandleFunc("POST /v1/runs", s.handleSubmit)
	mux.HandleFunc("GET /v1/runs", func(w http.ResponseWriter, r *http.Request) {
		requestID := r.URL.Query().Get("request_id")
		if requestID == "" {
			writeError(w, automation.NewError(automation.CodeBadRequest, "request_id query parameter is required"))

			return
		}

		run, e := s.RunFor(requestID)
		respond(w, http.StatusOK, run, e)
	})
	mux.HandleFunc("GET /v1/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		run, e := s.Run(r.PathValue("id"))
		respond(w, http.StatusOK, run, e)
	})
	mux.HandleFunc("POST /v1/runs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		run, e := s.Cancel(r.PathValue("id"))
		respond(w, http.StatusAccepted, run, e)
	})
	mux.HandleFunc("POST /v1/runs/{id}/inputs/{input}", func(w http.ResponseWriter, r *http.Request) {
		var answer automation.InputAnswer

		if !decode(w, r, &answer) {
			return
		}

		respond(w, http.StatusOK, struct{}{}, s.Answer(r.PathValue("id"), r.PathValue("input"), answer))
	})
	mux.HandleFunc("GET /v1/journal", s.handleJournal)
	mux.HandleFunc("GET /v1/events", s.handleEvents)
	mux.HandleFunc("POST /v1/shutdown", func(w http.ResponseWriter, _ *http.Request) {
		select {
		case s.requested <- "request":
		default:
		}

		writeJSON(w, http.StatusAccepted, struct{}{})
	})

	return s.authenticate(mux)
}

// authenticate requires the server token on every request.
func (s *Server) authenticate(next http.Handler) http.Handler {
	want := []byte("Bearer " + s.opts.Token)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), want) != 1 {
			writeError(w, automation.NewError(automation.CodeUnauthorized, "missing or invalid bearer token"))

			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	var req automation.RunRequest

	if !decode(w, r, &req) {
		return
	}

	run, replay, e := s.Submit(req)
	if e != nil {
		writeError(w, e)

		return
	}

	status := http.StatusAccepted
	if replay {
		w.Header().Set("Idempotent-Replay", "true")

		status = http.StatusOK
	}

	writeJSON(w, status, run)
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	after, e := intQuery(r, "after", 0)
	if e != nil {
		writeError(w, e)

		return
	}

	messages, e := s.Messages(int(after), r.URL.Query().Get("thinking") == "true")
	respond(w, http.StatusOK, messages, e)
}

// handleJournal returns durable events after a cursor, long-polling up to wait seconds.
func (s *Server) handleJournal(w http.ResponseWriter, r *http.Request) {
	after, e := intQuery(r, "after", 0)
	if e == nil {
		var limit, wait int64

		if limit, e = intQuery(r, "limit", 500); e == nil {
			if wait, e = intQuery(r, "wait", 0); e == nil {
				switch {
				case limit < 1 || limit > 5000:
					e = automation.NewError(automation.CodeBadRequest, "limit must be between 1 and 5000")
				case wait < 0 || wait > 60:
					e = automation.NewError(automation.CodeBadRequest, "wait must be between 0 and 60 seconds")
				default:
					s.pollJournal(w, r, after, int(limit), time.Duration(wait)*time.Second)

					return
				}
			}
		}
	}

	writeError(w, e)
}

func (s *Server) pollJournal(w http.ResponseWriter, r *http.Request, after int64, limit int, wait time.Duration) {
	deadline := time.NewTimer(wait)
	defer deadline.Stop()

	for {
		events, last, changed, err := s.journal.Read(after, limit)
		if err != nil {
			writeError(w, Classify(err))

			return
		}

		if len(events) > 0 || wait == 0 {
			if events == nil {
				events = []automation.Event{}
			}

			writeJSON(w, http.StatusOK, automation.Journal{Events: events, LastSeq: last})

			return
		}

		select {
		case <-changed:
		case <-deadline.C:
			wait = 0
		case <-s.closing:
			wait = 0
		case <-r.Context().Done():
			return
		}
	}
}

// handleEvents streams durable events as SSE, replaying from the cursor; optionally
// interleaves transient events (unsequenced and droppable).
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, automation.NewError(automation.CodeInternal, "streaming unsupported"))

		return
	}

	after, e := intQuery(r, "after", 0)
	if id := r.Header.Get("Last-Event-ID"); id != "" && e == nil {
		after, e = parseInt("Last-Event-ID", id)
	}

	if e != nil {
		writeError(w, e)

		return
	}

	if _, _, _, err := s.journal.Read(after, 0); err != nil {
		writeError(w, Classify(err))

		return
	}

	heartbeat, e := intQuery(r, "heartbeat", int64(defaultHeartbeat/time.Second))
	if e == nil && (heartbeat < 1 || heartbeat > 60) {
		e = automation.NewError(automation.CodeBadRequest, "heartbeat must be between 1 and 60 seconds")
	}

	if e != nil {
		writeError(w, e)

		return
	}

	query := r.URL.Query()
	thinking := query.Get("thinking") == "true"

	var live <-chan transient

	if query.Get("transient") == "true" {
		ch, unsubscribe := s.live.subscribe()
		defer unsubscribe()

		live = ch
	}

	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ticker := time.NewTicker(time.Duration(heartbeat) * time.Second)
	defer ticker.Stop()

	for {
		events, _, changed, err := s.journal.Read(after, 500)
		if err != nil {
			return
		}

		for _, event := range events {
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", event.Seq, event.Type, data)

			after = event.Seq
		}

		if len(events) > 0 {
			flusher.Flush()

			continue
		}

		select {
		case <-changed:
		case t := <-live:
			if t.Type == automation.EventDeltaThinking && !thinking {
				continue
			}

			data, _ := json.Marshal(t.Data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", t.Type, data)
			flusher.Flush()
		case <-ticker.C:
			// An event, not a comment, so clients can act (check for stop) while the stream is quiet.
			fmt.Fprintf(w, "event: %s\ndata: {}\n\n", automation.EventHeartbeat)
			flusher.Flush()
		case <-s.closing:
			return
		case <-r.Context().Done():
			return
		}
	}
}

// intQuery parses an integer query parameter.
func intQuery(r *http.Request, name string, fallback int64) (int64, *automation.Error) {
	value := r.URL.Query().Get(name)
	if value == "" {
		return fallback, nil
	}

	return parseInt(name, value)
}

func parseInt(name, value string) (int64, *automation.Error) {
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, automation.NewError(automation.CodeBadRequest, "%s must be an integer", name)
	}

	return n, nil
}

// decode reads a strict JSON body, writing a bad_request error on failure.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<20))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(v); err != nil {
		writeError(w, automation.NewError(automation.CodeBadRequest, "invalid request body: %v", err))

		return false
	}

	return true
}

// respond writes v with status, or the error.
func respond(w http.ResponseWriter, status int, v any, e *automation.Error) {
	if e != nil {
		writeError(w, e)

		return
	}

	writeJSON(w, status, v)
}

// statusFor maps error codes to HTTP status codes.
func statusFor(code automation.Code) int {
	switch code {
	case automation.CodeBadRequest, automation.CodeInvalidID:
		return http.StatusBadRequest
	case automation.CodeUnauthorized:
		return http.StatusUnauthorized
	case automation.CodeNotFound:
		return http.StatusNotFound
	case automation.CodeBusy, automation.CodeRequestConflict, automation.CodeCursorOutOfRange, automation.CodeNotPending:
		return http.StatusConflict
	case automation.CodeShuttingDown:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

func writeError(w http.ResponseWriter, e *automation.Error) {
	writeJSON(w, statusFor(e.Code), automation.Envelope{Error: e})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		status, data = http.StatusInternalServerError, []byte(`{"error":{"code":"internal","message":"encoding response","retryable":false,"details":{}}}`)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(append(data, '\n'))
}

