package core

import "github.com/idelchi/aura/internal/agent"

// Selection supplies initial session defaults without changing which CLI flags were set.
// Explicit flags take precedence; omitted values use the configured agent defaults.
type Selection struct {
	// Agent is the initial agent, for example from a task definition.
	Agent string
	// Mode is the initial mode when no mode flag was supplied.
	Mode string
}

// overrides resolves invocation settings with explicit flags above selection defaults.
func (s Selection) overrides(flags Flags) (agent.Overrides, error) {
	o, err := buildOverrides(flags)
	if err != nil {
		return o, err
	}
	if o.Agent == nil && s.Agent != "" {
		o.Agent = &s.Agent
	}
	if o.Mode == nil && s.Mode != "" {
		o.Mode = &s.Mode
	}
	return o, nil
}
