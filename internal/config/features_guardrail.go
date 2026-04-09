package config

import "time"

// Guardrail holds configuration for the guardrail feature — a secondary LLM that validates
// tool calls and user input against a policy prompt.
// Each scope (tool_calls, user_messages) has its own agent/prompt fields.
type Guardrail struct {
	// Mode controls what happens when the guardrail flags something.
	//   "block" — reject the tool call / message
	//   "log"   — show info note inline, proceed normally
	// Empty string = disabled (no guardrail checks run). Default.
	Mode string `validate:"omitempty,oneof=block log" yaml:"mode"`
	// OnError controls what happens when the guardrail check itself fails
	// (timeout, network error, model unavailable — after retries).
	//   "block" — fail-closed: block the content (default for mode=block)
	//   "allow" — fail-open: proceed without guardrail check (default for mode=log)
	// Decoupled from Mode so users can have mode=block + on_error=allow.
	OnError string `validate:"omitempty,oneof=block allow" yaml:"on_error"`
	// Timeout is the max duration for a single guardrail check.
	// If exceeded, the check is treated as if the guardrail model is unavailable.
	Timeout time.Duration `yaml:"timeout"`
	// Scope controls which message types are sent to the guardrail model.
	// Each scope has its own agent/prompt — a scope is active when agent or prompt is set.
	Scope GuardrailScope `yaml:"scope"`
	// Tools filters which tools trigger guardrail checks (for tool_calls scope).
	// Same enabled/disabled glob syntax as tools feature config.
	Tools GuardrailTools `yaml:"tools"`
}

// GuardrailScope controls which message types are sent to the guardrail model.
type GuardrailScope struct {
	// ToolCalls checks assistant-generated tool calls before execution.
	ToolCalls GuardrailScopeEntry `yaml:"tool_calls"`
	// UserMessages checks user input before it enters conversation.
	UserMessages GuardrailScopeEntry `yaml:"user_messages"`
}

// GuardrailScopeEntry configures a single guardrail scope with its own agent/prompt.
type GuardrailScopeEntry struct {
	// Agent is the dedicated guardrail agent name for this scope.
	Agent string `yaml:"agent"`
	// Prompt is a named system prompt (from prompts/). When set, uses the
	// current agent's provider/model with this prompt instead of a dedicated
	// agent. Overrides Agent (same semantics as compaction.prompt).
	Prompt string `yaml:"prompt"`
}

// Active reports whether this scope entry is configured (has agent or prompt).
func (e GuardrailScopeEntry) Active() bool {
	return e.Agent != "" || e.Prompt != ""
}

// GuardrailTools filters which tools trigger guardrail checks.
type GuardrailTools struct {
	// Enabled is a list of glob patterns. Only matching tools are checked.
	// Empty = all tools checked.
	Enabled []string `yaml:"enabled"`
	// Disabled is a list of glob patterns. Matching tools are skipped.
	// Applied after Enabled.
	Disabled []string `yaml:"disabled"`
}

// Enabled reports whether guardrail checks are active.
func (g Guardrail) Enabled() bool {
	return g.Mode != ""
}

// ApplyDefaults sets sane defaults for zero-valued fields.
func (g *Guardrail) ApplyDefaults() error {
	if g.Timeout == 0 && g.Enabled() {
		g.Timeout = 2 * time.Minute
	}

	if g.OnError == "" && g.Enabled() {
		if g.Mode == "block" {
			g.OnError = "block"
		} else {
			g.OnError = "allow"
		}
	}

	return nil
}
