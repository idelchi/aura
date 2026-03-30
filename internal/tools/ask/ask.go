// Package ask provides a tool for the LLM to ask the user questions mid-execution.
package ask

import (
	"context"
	"errors"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/pkg/llm/tool"
)

// Option represents a selectable choice presented to the user.
type Option struct {
	Label       string `json:"label"                 jsonschema:"required,description=Short display text for this choice"`
	Description string `json:"description,omitempty" jsonschema:"description=Explanation of what this option means"`
}

// Inputs defines the parameters for the Ask tool.
type Inputs struct {
	Question    string   `json:"question"               jsonschema:"required,description=The question to ask the user"`
	Options     []Option `json:"options,omitempty"      jsonschema:"description=Available choices (omit for free-form text input)"`
	MultiSelect bool     `json:"multi_select,omitempty" jsonschema:"description=Allow selecting multiple options (only with options)"`
}

// Request is the callback payload passed to the UI layer.
// Decoupled from JSON schema tags.
type Request struct {
	Question    string
	Options     []Option
	MultiSelect bool
}

// Tool allows the LLM to ask the user a question and receive a response.
type Tool struct {
	tool.Base

	ask func(ctx context.Context, req Request) (string, error)
}

// New creates an Ask tool with the given callback.
func New(ask func(ctx context.Context, req Request) (string, error)) *Tool {
	return &Tool{
		Base: tool.Base{
			Text: tool.Text{
				Description: heredoc.Doc(`
					Ask the user a question and wait for their response.
					Use this when you need clarification, confirmation, or a choice from the user.
					Without options, the user provides free-form text.
					With options, the user selects from the provided choices.
				`),
				Usage: heredoc.Doc(`
					Use this tool when you need user input before proceeding:
					- Clarifying ambiguous requirements
					- Confirming destructive or irreversible actions
					- Choosing between multiple valid approaches
					- Getting preferences or configuration values
				`),
				Examples: `{"question": "Which database should we use?", "options": [{"label": "PostgreSQL", "description": "Relational, ACID-compliant"}, {"label": "SQLite", "description": "Embedded, zero-config"}]}`,
			},
		},
		ask: ask,
	}
}

// Name returns the tool's identifier.
func (t *Tool) Name() string {
	return "Ask"
}

// Schema returns the provider-agnostic tool definition.
func (t *Tool) Schema() tool.Schema {
	return tool.GenerateSchema[Inputs](t)
}

// Execute asks the user a question via the callback and returns their response.
func (t *Tool) Execute(ctx context.Context, args map[string]any) (string, error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return "", err
	}

	if params.Question == "" {
		return "", errors.New("question is required")
	}

	req := Request{
		Question:    params.Question,
		Options:     params.Options,
		MultiSelect: params.MultiSelect,
	}

	return t.ask(ctx, req)
}

// Sandboxable returns false as this tool has no filesystem operations.
func (t *Tool) Sandboxable() bool {
	return false
}

// Parallel returns false because Ask blocks on user input; concurrent prompts would confuse the user.
func (t *Tool) Parallel() bool {
	return false
}
