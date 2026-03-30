// Package skill provides a meta-tool for LLM-invocable skills with progressive disclosure.
package skill

import (
	"context"
	"fmt"
	"strings"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/pkg/llm/tool"
)

// Inputs defines the parameters for the Skill tool.
type Inputs struct {
	Name string `json:"name" jsonschema:"required,description=Skill name to invoke"`
}

// Tool is a meta-tool that lets the LLM invoke user-defined skills by name.
// Skill descriptions are embedded in the tool's description for progressive disclosure.
// The full skill body is returned only when the LLM invokes a specific skill.
type Tool struct {
	tool.Base

	skills config.Collection[config.Skill]
}

// New creates a Skill tool with a dynamic description built from all skill descriptions.
func New(skills config.Collection[config.Skill]) *Tool {
	var desc strings.Builder
	desc.WriteString(heredoc.Doc(`
		Invoke a skill by name. Skills are multi-step workflows you execute using your existing tools.
		Call with the skill name. The skill's full instructions will be returned.
	`))

	desc.WriteString("\nAvailable skills:\n")

	for _, name := range skills.Names() {
		skill := skills.Get(name)

		fmt.Fprintf(&desc, "- %s: %s\n", skill.Metadata.Name, skill.Metadata.Description)
	}

	return &Tool{
		Base: tool.Base{
			Text: tool.Text{
				Description: strings.TrimSpace(desc.String()),
				Examples:    `{"name": "commit"}`,
			},
		},
		skills: skills,
	}
}

// Name returns the tool's identifier.
func (t *Tool) Name() string {
	return "Skill"
}

// Schema returns the provider-agnostic tool definition.
func (t *Tool) Schema() tool.Schema {
	return tool.GenerateSchema[Inputs](t)
}

// Execute looks up a skill by name and returns the full instructions.
func (t *Tool) Execute(_ context.Context, args map[string]any) (string, error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return "", err
	}

	skill := t.skills.Get(params.Name)
	if skill == nil {
		return "", fmt.Errorf("unknown skill %q, available: %s", params.Name, strings.Join(t.skills.Names(), ", "))
	}

	return skill.Body, nil
}

// Sandboxable returns false as this tool has no filesystem operations.
func (t *Tool) Sandboxable() bool {
	return false
}
