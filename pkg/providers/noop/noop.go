// Package noop provides a no-operation LLM provider that returns fixed responses
// without making any network calls. Used by --dry=noop to exercise the full
// pipeline end-to-end without a running model.
package noop

import (
	"context"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/stream"
	"github.com/idelchi/aura/pkg/llm/usage"
)

// Provider is a no-operation LLM provider. Chat returns a fixed "[noop]" message
// with no tool calls, causing the assistant loop to exit after one turn.
type Provider struct{}

// New creates a noop provider.
func New() *Provider { return &Provider{} }

// Chat returns a fixed "[noop]" assistant message with no tool calls and zero usage.
func (p *Provider) Chat(
	_ context.Context,
	_ request.Request,
	streamFunc stream.Func,
) (message.Message, usage.Usage, error) {
	if streamFunc != nil {
		if err := streamFunc("", "[noop]", false); err != nil {
			return message.Message{}, usage.Usage{}, err
		}

		if err := streamFunc("", "", true); err != nil {
			return message.Message{}, usage.Usage{}, err
		}
	}

	return message.Message{
		Role:    roles.Assistant,
		Content: "[noop]",
	}, usage.Usage{}, nil
}

// Models returns a single noop:latest model entry.
func (p *Provider) Models(_ context.Context) (model.Models, error) {
	return model.Models{
		{Name: "noop:latest"},
	}, nil
}

// Model returns a minimal model entry with the requested name.
func (p *Provider) Model(_ context.Context, name string) (model.Model, error) {
	return model.Model{Name: name}, nil
}
