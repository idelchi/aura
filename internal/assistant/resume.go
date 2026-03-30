package assistant

import (
	"context"
	"fmt"

	"github.com/idelchi/aura/internal/session"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/thinking"
)

// ResumeSession restores the assistant state from a saved session.
// Returns any non-fatal warnings encountered during restoration.
func (a *Assistant) ResumeSession(ctx context.Context, sess *session.Session) []string {
	var warnings []string

	if a.cliOverrides.Agent != nil {
		// ── CLI agent override: replaces session agent entirely ──
		// SwitchAgent constructs with cliOverrides (mode/think/model/provider).
		// No session mode/think/model restoration needed.
		if err := a.SwitchAgent(*a.cliOverrides.Agent, "user"); err != nil {
			warnings = append(warnings, fmt.Sprintf("could not apply --agent override: %v", err))
		}
	} else {
		// ── No agent override: restore from session, then apply individual overrides ──

		// Restore agent
		if sess.Meta.Agent != "" {
			if err := a.SwitchAgent(sess.Meta.Agent, "resume"); err != nil {
				warnings = append(warnings, fmt.Sprintf("could not restore agent %q", sess.Meta.Agent))
			}
		}

		// Restore mode (cheap — field assignment + rebuildState)
		if sess.Meta.Mode != "" {
			if err := a.SwitchMode(sess.Meta.Mode); err != nil {
				warnings = append(warnings, fmt.Sprintf("could not restore mode %q", sess.Meta.Mode))
			}
		}

		// Restore thinking (cheap — field assignment + rebuildState)
		if sess.Meta.Think != "" {
			if err := a.SetThink(thinking.NewValue(parseThinkValue(sess.Meta.Think))); err != nil {
				warnings = append(warnings, fmt.Sprintf("could not restore think %q", sess.Meta.Think))
			}
		}

		// Restore model/provider — skip if CLI will override (avoids redundant LoadModel)
		hasModelOverride := a.cliOverrides.Model != nil || a.cliOverrides.Provider != nil
		if sess.Meta.Model != "" && sess.Meta.Provider != "" && !hasModelOverride {
			r := a.resolved.config
			if sess.Meta.Model != r.Model || sess.Meta.Provider != r.Provider {
				if err := a.SwitchModel(ctx, sess.Meta.Provider, sess.Meta.Model); err != nil {
					warnings = append(
						warnings,
						fmt.Sprintf("could not restore model %s/%s", sess.Meta.Provider, sess.Meta.Model),
					)
				}
			}
		}

		// Apply CLI overrides on top of restored session state.
		// Priority: CLI > Session > Agent config.
		if a.cliOverrides.Mode != nil {
			if err := a.SwitchMode(*a.cliOverrides.Mode); err != nil {
				warnings = append(warnings, fmt.Sprintf("could not apply --mode override: %v", err))
			}
		}

		if a.cliOverrides.Think != nil {
			if err := a.SetThink(*a.cliOverrides.Think); err != nil {
				warnings = append(warnings, fmt.Sprintf("could not apply --think override: %v", err))
			}
		}

		if a.cliOverrides.Provider != nil || a.cliOverrides.Model != nil {
			// Compose from session baseline + CLI overrides.
			provider := sess.Meta.Provider
			if provider == "" {
				provider = a.resolved.config.Provider
			}

			modelName := sess.Meta.Model
			if modelName == "" {
				modelName = a.resolved.config.Model
			}

			if a.cliOverrides.Provider != nil {
				provider = *a.cliOverrides.Provider
			}

			if a.cliOverrides.Model != nil {
				modelName = *a.cliOverrides.Model
			}

			if err := a.SwitchModel(ctx, provider, modelName); err != nil {
				warnings = append(warnings, fmt.Sprintf("could not apply model/provider override: %v", err))
			}
		}
	}

	// Restore show-thinking UI preference
	a.SetVerbose(sess.Meta.Verbose)

	// Restore read-before policy if user changed it at runtime
	if sess.Meta.ReadBeforePolicy != nil {
		if err := a.SetReadBeforePolicy(*sess.Meta.ReadBeforePolicy); err != nil {
			warnings = append(warnings, fmt.Sprintf("could not restore read-before policy: %v", err))
		}
	}

	// Restore runtime toggles
	if sess.Meta.Sandbox {
		if err := a.SetSandbox(true); err != nil {
			warnings = append(warnings, fmt.Sprintf("could not restore sandbox: %v", err))
		}
	}

	// Restore session approvals
	if sess.Meta.SessionApprovals != nil {
		a.session.approvals = sess.Meta.SessionApprovals
	}

	// Restore stats (preserves original start time and all counters)
	if sess.Meta.Stats != nil {
		a.session.stats = sess.Meta.Stats
		// Guard nil map after JSON deserialization
		if a.session.stats.Tools.Freq == nil {
			a.session.stats.Tools.Freq = make(map[string]int)
		}
	}

	// Restore cumulative usage
	if sess.Meta.CumulativeUsage != nil {
		a.session.usage = *sess.Meta.CumulativeUsage
	}

	// Restore messages — replace system prompt with current one
	systemTokens := a.estimator.Estimate(ctx, a.agent.Prompt)
	restored := restoreMessages(a.agent.Prompt, systemTokens, sess.Messages)
	a.builder.Restore(restored)

	// Emit displayable messages to the UI so the TUI can render the restored
	// conversation. Parts are already reconstructed by Message.UnmarshalJSON.
	a.send(ui.SessionRestored{Messages: restored.ForDisplay()})

	// Mark dirty so AutoSave persists the resumed session
	a.session.dirty = true

	// Restore loaded tools from session.
	if len(sess.Meta.LoadedTools) > 0 {
		for _, name := range sess.Meta.LoadedTools {
			a.tools.loaded[name] = true
		}

		a.rt.LoadedTools = a.tools.loaded

		if err := a.rebuildState(); err != nil {
			warnings = append(warnings, fmt.Sprintf("could not restore loaded tools: %v", err))
		}
	}

	// Restore todos
	if sess.Todos != nil {
		a.tools.todo.SetSummary(sess.Todos.Summary)
		a.tools.todo.Replace(sess.Todos.Get())
	}

	return warnings
}

// restoreMessages rebuilds the message history using the current system prompt
// and the saved conversation messages (skipping the old system prompt).
func restoreMessages(currentPrompt string, systemTokens int, saved message.Messages) message.Messages {
	restored := message.New(message.Message{
		Role:    roles.System,
		Content: currentPrompt,
		Tokens:  message.Tokens{Total: systemTokens},
	})

	for _, msg := range saved {
		if msg.Role == roles.System {
			continue
		}

		restored.Add(msg)
	}

	return restored
}

// parseThinkValue converts a stored string back to a ThinkValue-compatible value.
func parseThinkValue(s string) any {
	switch s {
	case "true":
		return true
	case "false":
		return false
	default:
		return s
	}
}
