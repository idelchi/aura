// Package script provides a deterministic provider that replays a scripted sequence of
// responses, tool calls, delays and typed failures. It exists to test automation clients
// and lifecycle behaviour through the real engine without a model.
//
// The replay position is persisted next to the script (<script>.cursor), so a run that
// is killed and resumed in a new process continues with the next step. A step is
// consumed when it starts.
package script

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/idelchi/aura/internal/atomicfile"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/stream"
	"github.com/idelchi/aura/pkg/llm/tool/call"
	"github.com/idelchi/aura/pkg/llm/usage"
	"github.com/idelchi/aura/pkg/providers"
)

// ErrExhausted is returned when every step of the script has been replayed.
var ErrExhausted = errors.New("script exhausted")

// Model is a model the script advertises.
type Model struct {
	Name    string `json:"name"`
	Context int    `json:"context"`
}

// Call is a scripted tool call.
type Call struct {
	ID   string         `json:"id"`
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

// Failure is a scripted provider failure.
type Failure struct {
	// Class is one of network, server, rate_limit, auth, credit, model_unavailable, content_filter, context.
	Class      string `json:"class"`
	Status     int    `json:"status"`
	Message    string `json:"message"`
	RetryAfter string `json:"retry_after"`
}

// Step is one scripted chat response.
type Step struct {
	Delay    string       `json:"delay"` // wait before responding
	Thinking string       `json:"thinking"`
	Content  string       `json:"content"`
	Stall    string       `json:"stall"` // wait after streaming, before returning
	Calls    []Call       `json:"calls"`
	Error    *Failure     `json:"error"`
	Usage    *usage.Usage `json:"usage"`
	// Finish is the reported finish reason (message.Finish values, or "unreported").
	// Empty reports "tool_calls" for a step with calls and "stop" otherwise: a replayed
	// step is complete by definition.
	Finish string `json:"finish"`
}

// Script is the content of a script file.
type Script struct {
	Models []Model `json:"models"`
	Steps  []Step  `json:"steps"`
	// MetadataError, when set, fails every model metadata request (Models, Model) with it.
	MetadataError *Failure `json:"metadata_error"`
}

// Provider replays a script.
type Provider struct {
	path   string
	script Script
	mu     sync.Mutex
}

// New loads and validates the script at path.
func New(path string) (*Provider, error) {
	if path == "" {
		return nil, errors.New("script provider requires a script path")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading script: %w", err)
	}

	var script Script

	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&script); err != nil {
		return nil, fmt.Errorf("parsing script %s: %w", path, err)
	}

	for i, step := range script.Steps {
		if err := step.validate(); err != nil {
			return nil, fmt.Errorf("script %s step %d: %w", path, i+1, err)
		}
	}

	if script.MetadataError != nil {
		if err := (Step{Error: script.MetadataError}).validate(); err != nil {
			return nil, fmt.Errorf("script %s metadata_error: %w", path, err)
		}
	}

	return &Provider{path: path, script: script}, nil
}

// finish returns the finish reason this step reports.
func (s Step) finish() message.Finish {
	switch {
	case s.Finish == "unreported":
		return message.FinishUnreported
	case s.Finish != "":
		return message.Finish(s.Finish)
	case len(s.Calls) > 0:
		return message.FinishToolCalls
	default:
		return message.FinishStop
	}
}

// validate checks durations, finish reasons and failure classes.
func (s Step) validate() error {
	switch message.Finish(s.Finish) {
	case "", "unreported", message.FinishStop, message.FinishLength, message.FinishToolCalls,
		message.FinishContentFilter, message.FinishError, message.FinishOther:
	default:
		return fmt.Errorf("unknown finish %q", s.Finish)
	}

	for _, d := range []string{s.Delay, s.Stall} {
		if d == "" {
			continue
		}

		if _, err := time.ParseDuration(d); err != nil {
			return err
		}
	}

	if s.Error == nil {
		return nil
	}

	if s.Error.RetryAfter != "" {
		if _, err := time.ParseDuration(s.Error.RetryAfter); err != nil {
			return err
		}
	}

	switch s.Error.Class {
	case "network", "server", "rate_limit", "auth", "credit", "model_unavailable", "content_filter", "context":
		return nil
	default:
		return fmt.Errorf("unknown failure class %q", s.Error.Class)
	}
}

// next consumes and returns the next step, persisting the replay position.
func (p *Provider) next() (Step, int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	cursorPath := p.path + ".cursor"

	cursor := 0

	data, err := os.ReadFile(cursorPath)

	switch {
	case err == nil:
		cursor, err = strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			return Step{}, 0, fmt.Errorf("reading script cursor: %w", err)
		}
	case !errors.Is(err, fs.ErrNotExist):
		return Step{}, 0, fmt.Errorf("reading script cursor: %w", err)
	}

	if cursor >= len(p.script.Steps) {
		return Step{}, cursor, fmt.Errorf("%w after %d steps", ErrExhausted, len(p.script.Steps))
	}

	if err := atomicfile.WriteFile(cursorPath, []byte(strconv.Itoa(cursor+1))); err != nil {
		return Step{}, cursor, fmt.Errorf("writing script cursor: %w", err)
	}

	return p.script.Steps[cursor], cursor, nil
}

// wait sleeps for d, returning early with the context's error.
func wait(ctx context.Context, d string) error {
	if d == "" {
		return nil
	}

	duration, _ := time.ParseDuration(d)

	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Chat replays the next step.
func (p *Provider) Chat(ctx context.Context, _ request.Request, streamFunc stream.Func) (message.Message, usage.Usage, error) {
	if err := ctx.Err(); err != nil {
		return message.Message{}, usage.Usage{}, err
	}

	step, index, err := p.next()
	if err != nil {
		return message.Message{}, usage.Usage{}, err
	}

	if err := wait(ctx, step.Delay); err != nil {
		return message.Message{}, usage.Usage{}, err
	}

	if step.Error != nil {
		return message.Message{}, usage.Usage{}, step.Error.err()
	}

	if streamFunc != nil {
		if step.Thinking != "" {
			if err := streamFunc(step.Thinking, "", false); err != nil {
				return message.Message{}, usage.Usage{}, err
			}
		}

		if step.Content != "" {
			if err := streamFunc("", step.Content, false); err != nil {
				return message.Message{}, usage.Usage{}, err
			}
		}
	}

	if err := wait(ctx, step.Stall); err != nil {
		return message.Message{}, usage.Usage{}, err
	}

	if streamFunc != nil {
		if err := streamFunc("", "", true); err != nil {
			return message.Message{}, usage.Usage{}, err
		}
	}

	msg := message.Message{Role: roles.Assistant, Content: step.Content, Thinking: step.Thinking, Finish: step.finish()}

	for i, c := range step.Calls {
		id := c.ID
		if id == "" {
			id = fmt.Sprintf("script-%d-%d", index+1, i+1)
		}

		args := c.Args
		if args == nil {
			args = map[string]any{}
		}

		msg.Calls = append(msg.Calls, call.Call{ID: id, Name: c.Name, Arguments: args})
	}

	u := usage.Usage{}
	if step.Usage != nil {
		u = *step.Usage
	}

	return msg, u, nil
}

// err builds the provider error of a failure, classified like real provider errors.
func (f Failure) err() error {
	retryAfter, _ := time.ParseDuration(f.RetryAfter)

	status := func(code int) int {
		if f.Status != 0 {
			return f.Status
		}

		return code
	}

	switch f.Class {
	case "network":
		return providers.WrapNetworkError(errors.New(f.Message))
	case "server":
		return providers.ClassifyHTTPError(status(503), "script", f.Message, 0)
	case "rate_limit":
		return providers.ClassifyHTTPError(status(429), "script", f.Message, retryAfter)
	case "auth":
		return providers.ClassifyHTTPError(status(401), "script", f.Message, 0)
	case "credit":
		return fmt.Errorf("%w: %w", providers.ErrCreditExhausted,
			&providers.StatusError{Provider: "script", Status: status(402), Message: f.Message})
	case "model_unavailable":
		return providers.ClassifyHTTPError(status(404), "script", f.Message, 0)
	case "content_filter":
		return fmt.Errorf("%w: %s", providers.ErrContentFilter, f.Message)
	default: // context
		return fmt.Errorf("%w: %s", providers.ErrContextExhausted, f.Message)
	}
}

// Models returns the scripted models.
func (p *Provider) Models(_ context.Context) (model.Models, error) {
	if p.script.MetadataError != nil {
		return nil, p.script.MetadataError.err()
	}

	models := make(model.Models, 0, len(p.script.Models))

	for _, m := range p.script.Models {
		models = append(models, model.Model{Name: m.Name, ContextLength: model.ContextLength(m.Context)})
	}

	return models, nil
}

// Model returns the scripted model with the given name.
func (p *Provider) Model(_ context.Context, name string) (model.Model, error) {
	if p.script.MetadataError != nil {
		return model.Model{}, p.script.MetadataError.err()
	}

	for _, m := range p.script.Models {
		if m.Name == name {
			return model.Model{Name: m.Name, ContextLength: model.ContextLength(m.Context)}, nil
		}
	}

	return model.Model{}, fmt.Errorf("%w: script has no model %q", providers.ErrModelUnavailable, name)
}

// Estimate is not supported by the script provider.
func (p *Provider) Estimate(_ context.Context, _ request.Request, _ string) (int, error) {
	return 0, providers.ErrEstimateNotSupported
}
