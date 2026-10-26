package assistant

import (
	"context"
	"errors"
	"fmt"

	"github.com/idelchi/aura/internal/session"
	"github.com/idelchi/aura/internal/stats"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/thinking"
)

// ResumeSession restores the assistant state from a saved session.
//
// Session-scoped state (conversation, file reads, loaded tools, todos, approvals, usage,
// stats and tool budgets) is reset first, so nothing leaks from a previously active
// session. Every part of the stored state must restore: any failure is returned, and the
// assistant is then reset to a fresh, detached session so the half-restored state can
// never be saved over the stored one.
func (a *Assistant) ResumeSession(ctx context.Context, sess *session.Session) error {
	if err := a.resetSession(ctx); err != nil {
		return err
	}

	if err := a.restoreSession(ctx, sess); err != nil {
		if a.session.manager != nil {
			a.session.manager.Fork()
		}

		return errors.Join(err, a.resetSession(ctx))
	}

	return nil
}

// restoreSession applies the stored state of sess on top of a reset assistant.
func (a *Assistant) restoreSession(ctx context.Context, sess *session.Session) error {
	var errs []error

	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf(format, args...))
	}

	a.session.callLimits.Restore(sess.Meta.ToolCallUsage)

	if a.invocationOverrides.Agent != nil {
		// An invocation agent selection replaces the stored agent entirely.
		// Preserve invocation model/provider settings during initial restoration.
		// No session mode/think/model restoration needed.
		if err := a.switchAgent(*a.invocationOverrides.Agent, "resume", a.invocationOverrides); err != nil {
			fail("applying invocation agent selection: %w", err)
		}
	} else {
		// ── No agent override: restore from session, then apply individual overrides ──

		// Restore agent
		if sess.Meta.Agent != "" {
			if err := a.SwitchAgent(sess.Meta.Agent, "resume"); err != nil {
				fail("restoring agent %q: %w", sess.Meta.Agent, err)
			}
		}

		// Restore mode (cheap — field assignment + rebuildState)
		if sess.Meta.Mode != "" {
			if err := a.SwitchMode(sess.Meta.Mode); err != nil {
				fail("restoring mode %q: %w", sess.Meta.Mode, err)
			}
		}

		// Restore thinking (cheap — field assignment + rebuildState)
		if sess.Meta.Think != "" {
			if err := a.SetThink(thinking.NewValue(parseThinkValue(sess.Meta.Think))); err != nil {
				fail("restoring think %q: %w", sess.Meta.Think, err)
			}
		}

		// Restore model/provider — skip if CLI will override (avoids redundant LoadModel)
		hasModelOverride := a.invocationOverrides.Model != nil || a.invocationOverrides.Provider != nil
		if sess.Meta.Model != "" && sess.Meta.Provider != "" && !hasModelOverride {
			r := a.resolved.config
			if sess.Meta.Model != r.Model || sess.Meta.Provider != r.Provider {
				if err := a.SwitchModel(ctx, sess.Meta.Provider, sess.Meta.Model); err != nil {
					fail("restoring model %s/%s: %w", sess.Meta.Provider, sess.Meta.Model, err)
				}
			}
		}

		// Apply CLI overrides on top of restored session state.
		// Priority: CLI > Session > Agent config.
		if a.invocationOverrides.Mode != nil {
			if err := a.SwitchMode(*a.invocationOverrides.Mode); err != nil {
				fail("applying --mode override: %w", err)
			}
		}

		if a.invocationOverrides.Think != nil {
			if err := a.SetThink(*a.invocationOverrides.Think); err != nil {
				fail("applying --think override: %w", err)
			}
		}

		if a.invocationOverrides.Provider != nil || a.invocationOverrides.Model != nil {
			// Compose from session baseline + CLI overrides.
			provider := sess.Meta.Provider
			if provider == "" {
				provider = a.resolved.config.Provider
			}

			modelName := sess.Meta.Model
			if modelName == "" {
				modelName = a.resolved.config.Model
			}

			if a.invocationOverrides.Provider != nil {
				provider = *a.invocationOverrides.Provider
			}

			if a.invocationOverrides.Model != nil {
				modelName = *a.invocationOverrides.Model
			}

			if err := a.SwitchModel(ctx, provider, modelName); err != nil {
				fail("applying model/provider override: %w", err)
			}
		}
	}

	// Restore show-thinking UI preference
	a.SetVerbose(sess.Meta.Verbose)

	// Restore read-before policy if user changed it at runtime
	if sess.Meta.ReadBeforePolicy != nil {
		if err := a.SetReadBeforePolicy(*sess.Meta.ReadBeforePolicy); err != nil {
			fail("restoring read-before policy: %w", err)
		}
	}

	// Restore runtime toggles. A stored sandbox must be enforced again, not just requested.
	if sess.Meta.Sandbox {
		if err := a.SetSandbox(true); err != nil {
			fail("restoring sandbox: %w", err)
		} else if !a.toggles.sandbox {
			fail("restoring sandbox: Landlock is not available on this kernel")
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
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
			return fmt.Errorf("restoring loaded tools: %w", err)
		}
	}

	// Restore todos
	if sess.Todos != nil {
		a.tools.todo.SetSummary(sess.Todos.Summary)
		a.tools.todo.Replace(sess.Todos.Get())
	}

	return nil
}

// resetSession returns session-scoped state to that of a fresh conversation. Configuration,
// the selected agent and runtime toggles are kept.
func (a *Assistant) resetSession(ctx context.Context) error {
	a.builder.Clear()
	a.builder.UpdateSystemPrompt(ctx, a.SystemPrompt())
	a.ResetTokens()
	a.ResetToolCalls()
	a.tracker.Forget()
	a.tools.todo.SetSummary("")
	a.tools.todo.Replace(nil)
	clear(a.tools.loaded)
	a.rt.LoadedTools = a.tools.loaded
	a.session.approvals = make(map[string]bool)
	a.session.stats = stats.New()
	a.session.dirty = false

	return a.rebuildState()
}

// NewSession detaches from the stored session and starts a fresh, unsaved conversation.
// The next save creates a new session; the previous one stays exactly as last stored.
func (a *Assistant) NewSession(ctx context.Context) error {
	if a.session.manager != nil {
		a.session.manager.Fork()
	}

	return a.resetSession(ctx)
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
