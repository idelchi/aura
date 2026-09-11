package assistant

import (
	"errors"
	"fmt"

	"github.com/idelchi/aura/internal/injector"
)

// ErrPolicyStopped identifies an opt-in plugin ending a turn without completion.
var ErrPolicyStopped = errors.New("plugin policy stopped the turn")

// injectionStop applies explicit policy stops before any pending side effects.
func injectionStop(injections []injector.Injection) error {
	for _, inj := range injections {
		if inj.Stop != "" {
			return fmt.Errorf("%w: %s: %s", ErrPolicyStopped, inj.Name, inj.Stop)
		}
	}
	return nil
}

// availableToolNames includes turn restrictions and exhausted conversation budgets.
func (a *Assistant) availableToolNames() []string {
	names := a.loop.filterTools(a.agent.Tools).Names()
	available := names[:0]
	for _, name := range names {
		if !a.session.callLimits.Exhausted(name, a.cfg.Features.ToolExecution.CallLimits) {
			available = append(available, name)
		}
	}
	return available
}
