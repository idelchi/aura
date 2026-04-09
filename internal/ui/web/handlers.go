package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/idelchi/aura/internal/ui"
)

// parseField extracts a field from either JSON body or form-encoded body.
func parseField(r *http.Request, field string) string {
	// Try JSON first.
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/json") {
		var m map[string]string
		if json.NewDecoder(r.Body).Decode(&m) == nil {
			return m[field]
		}

		return ""
	}

	// Fall back to form values (htmx default).
	return r.FormValue(field)
}

func (w *Web) handleSend(wr http.ResponseWriter, r *http.Request) {
	text := parseField(r, "text")
	if text == "" {
		jsonError(wr, "empty text", http.StatusBadRequest)

		return
	}

	select {
	case w.input <- ui.UserInput{Text: text}:
		jsonOK(wr)
	case <-r.Context().Done():
		jsonError(wr, "request cancelled", http.StatusServiceUnavailable)
	}
}

func (w *Web) handleCancel(wr http.ResponseWriter, r *http.Request) {
	select {
	case w.cancel <- struct{}{}:
	default:
		// Already cancelled or nothing to cancel
	}

	jsonOK(wr)
}

func (w *Web) handleCommand(wr http.ResponseWriter, r *http.Request) {
	command := parseField(r, "command")
	if command == "" {
		jsonError(wr, "empty command", http.StatusBadRequest)

		return
	}

	// Send as slash command text (prefixed with /).
	text := "/" + command

	select {
	case w.input <- ui.UserInput{Text: text}:
		jsonOK(wr)
	case <-r.Context().Done():
		jsonError(wr, "request cancelled", http.StatusServiceUnavailable)
	}
}

func (w *Web) handleAsk(wr http.ResponseWriter, r *http.Request) {
	answer := parseField(r, "answer")

	w.mu.Lock()
	ch := w.pendingAsk
	options := w.pendingAskOptions
	multi := w.pendingAskMulti

	w.pendingAsk = nil
	w.pendingAskOptions = nil
	w.pendingAskMulti = false
	w.mu.Unlock()

	if ch == nil {
		jsonError(wr, "no pending ask", http.StatusConflict)

		return
	}

	resolved := ui.ResolveAskResponse(answer, options, multi)

	select {
	case ch <- resolved:
		// Clear the dialog.
		w.broker.Broadcast("ask", "")
		jsonOK(wr)
	case <-r.Context().Done():
		jsonError(wr, "request cancelled", http.StatusServiceUnavailable)
	}
}

func (w *Web) handleConfirm(wr http.ResponseWriter, r *http.Request) {
	actionStr := parseField(r, "action")

	w.mu.Lock()
	ch := w.pendingConfirm

	w.pendingConfirm = nil
	w.mu.Unlock()

	if ch == nil {
		jsonError(wr, "no pending confirm", http.StatusConflict)

		return
	}

	var action ui.ConfirmAction

	switch actionStr {
	case "allow":
		action = ui.ConfirmAllow
	case "allow_session":
		action = ui.ConfirmAllowSession
	case "allow_project":
		action = ui.ConfirmAllowPatternProject
	case "allow_global":
		action = ui.ConfirmAllowPatternGlobal
	case "deny":
		action = ui.ConfirmDeny
	default:
		action = ui.ConfirmDeny
	}

	select {
	case ch <- action:
		// Clear the dialog.
		w.broker.Broadcast("confirm", "")
		jsonOK(wr)
	case <-r.Context().Done():
		jsonError(wr, "request cancelled", http.StatusServiceUnavailable)
	}
}

func jsonOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	resp, _ := json.Marshal(map[string]string{"error": msg})
	w.Write(resp)
}
