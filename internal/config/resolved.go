package config

import (
	"github.com/idelchi/aura/pkg/llm/generation"
	"github.com/idelchi/aura/pkg/llm/thinking"
)

// Resolved holds the effective runtime configuration after all merge chains
// and runtime toggles have settled. Built at the end of rebuildState() and
// assistant.New(). Read by everything that needs "what is the assistant
// actually using right now."
//
// Does NOT include per-turn state (iteration, tool history, tokens),
// session state (ID, approvals, stats), filesystem paths, or tool lists.
type Resolved struct {
	// Agent identity.
	Agent    string
	Mode     string
	Provider string
	Model    string

	// Model configuration.
	Think      thinking.Value
	Context    int
	Generation *generation.Generation
	Thinking   thinking.Strategy

	// Effective features (post global → agent → mode → task → CLI override → runtime toggles).
	Features Features

	// Runtime toggles.
	Sandbox bool
	Verbose bool
	Auto    bool
	Done    bool
}
