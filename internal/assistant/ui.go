package assistant

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/todo"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/model"
)

// ContextLength returns the context window size to use for display
// and compaction. User-configured context always wins; otherwise falls
// back to the provider-reported value.
func (a *Assistant) ContextLength() model.ContextLength {
	// User-configured context always wins.
	if a.agent.Model.Context > 0 {
		resolved := a.resolved.model.Deref()

		debug.Log("[context] ContextLength: using agent override=%d (resolvedModel.ContextLength=%d)",
			a.agent.Model.Context, int(resolved.ContextLength))

		return model.ContextLength(a.agent.Model.Context)
	}

	// Otherwise use what the provider reported.
	resolved := a.resolved.model.Deref()

	debug.Log("[context] ContextLength: using resolvedModel=%d (agent.Context=%d)",
		int(resolved.ContextLength), a.agent.Model.Context)

	return resolved.ContextLength
}

// Status returns the current status for the UI.
func (a *Assistant) Status() ui.Status {
	r := a.resolved.config
	contextLen := a.ContextLength()
	tokensUsed := a.Tokens()

	return ui.Status{
		Agent:    r.Agent,
		Mode:     r.Mode,
		Provider: r.Provider,
		Model:    r.Model,
		Think:    r.Think,
		Tokens: struct {
			Used    int
			Max     int
			Percent float64
		}{
			Used:    tokensUsed,
			Max:     int(contextLen),
			Percent: contextLen.PercentUsed(tokensUsed),
		},
		Sandbox: struct {
			Enabled   bool
			Requested bool
		}{
			Enabled:   r.Sandbox,
			Requested: a.toggles.sandboxRequested,
		},
		Snapshots: a.tools.snapshots != nil,
		Steps: struct {
			Current int
			Max     int
		}{
			Current: a.loop.iteration,
			Max:     r.Features.ToolExecution.MaxSteps,
		},
	}
}

// EstimateTokens returns a client-side token estimate by summing per-message Tokens.Total
// for messages that affect context budget (excludes internal types) plus cached schema tokens.
// No re-rendering or provider calls — O(n) message count, not O(n) text.
func (a *Assistant) EstimateTokens(_ context.Context) int {
	return a.builder.History().TokensForEstimation() + a.resolved.schemaTokens
}

// EmitStatus sends the current system status to the UI.
func (a *Assistant) EmitStatus() {
	a.send(ui.StatusChanged{Status: a.Status()})
}

// DisplayHints returns the current UI display preferences.
func (a *Assistant) DisplayHints() ui.DisplayHints {
	return ui.DisplayHints{
		Verbose: a.toggles.verbose,
		Auto:    a.toggles.auto,
	}
}

// EmitDisplayHints sends the current display preferences to the UI.
func (a *Assistant) EmitDisplayHints() {
	a.send(ui.DisplayHintsChanged{Hints: a.DisplayHints()})
}

// EmitAll sends both system status and display hints to the UI.
func (a *Assistant) EmitAll() {
	a.EmitStatus()
	a.EmitDisplayHints()
}

// applyUIAction applies a UI-triggered action.
func (a *Assistant) applyUIAction(ctx context.Context, action ui.Action) {
	switch act := action.(type) {
	case ui.ToggleVerbose:
		a.ToggleVerbose()
	case ui.ToggleThink:
		if err := a.ToggleThink(); err != nil {
			a.send(ui.CommandResult{Command: "config", Error: err})
		}
	case ui.CycleThink:
		if err := a.CycleThink(); err != nil {
			a.send(ui.CommandResult{Command: "config", Error: err})
		}
	case ui.ToggleAuto:
		a.ToggleAuto()
	case ui.ToggleSandbox:
		if err := a.ToggleSandbox(); err != nil {
			a.send(ui.CommandResult{Command: "config", Error: err})
		}
	case ui.NextAgent:
		if err := a.NextAgent(); err != nil {
			a.send(ui.CommandResult{Command: "/agent", Error: err})
		}
	case ui.NextMode:
		if err := a.NextMode(); err != nil {
			a.send(ui.CommandResult{Command: "/mode", Error: err})
		}
	case ui.SelectModel:
		if err := a.SwitchModel(ctx, act.Provider, act.Model); err != nil {
			a.send(ui.CommandResult{Error: err})
		} else {
			a.send(
				ui.CommandResult{
					Message: fmt.Sprintf("Switched to %s/%s", act.Provider, act.Model),
					Level:   ui.LevelSuccess,
				},
			)
		}
	case ui.ResumeSession:
		if a.session.manager == nil {
			return
		}

		sess, err := a.session.manager.Resume(act.SessionID)
		if err != nil {
			a.send(ui.CommandResult{Error: err})

			return
		}

		warnings := a.ResumeSession(ctx, sess)

		result := "Resumed session: " + sess.ShortDisplay()

		if len(warnings) > 0 {
			result += " [" + strings.Join(warnings, "; ") + "]"
		}

		a.send(ui.CommandResult{Message: result, Level: ui.LevelSuccess})
	case ui.TodoEdited:
		parsed, err := todo.Parse(act.Text)
		if err != nil {
			a.send(ui.CommandResult{Command: "/todo edit", Error: err})

			return
		}

		a.tools.todo.SetSummary(parsed.Summary)
		a.tools.todo.Replace(parsed.Items)

		a.send(ui.CommandResult{Command: "/todo edit", Message: "Todo list updated.", Level: ui.LevelSuccess})
	case ui.UndoSnapshot:
		if act.Hash == "" {
			// No git — message-only rewind
			a.send(ui.PickerOpen{Title: "What to rewind?", Items: []ui.PickerItem{
				{
					Label:       "Messages only",
					Description: "Rewind conversation, keep current files",
					Action:      ui.UndoExecute{MessageIndex: act.MessageIndex, Mode: "messages"},
				},
			}})
		} else {
			// Full rewind options (code + messages)
			items := []ui.PickerItem{
				{
					Label:       "Code and messages",
					Description: "Restore files and rewind conversation",
					Action:      ui.UndoExecute{Hash: act.Hash, MessageIndex: act.MessageIndex, Mode: "both"},
				},
				{
					Label:       "Code only",
					Description: "Restore files, keep conversation intact",
					Action:      ui.UndoExecute{Hash: act.Hash, MessageIndex: act.MessageIndex, Mode: "code"},
				},
				{
					Label:       "Messages only",
					Description: "Rewind conversation, keep current files",
					Action:      ui.UndoExecute{Hash: act.Hash, MessageIndex: act.MessageIndex, Mode: "messages"},
				},
			}

			a.send(ui.PickerOpen{Title: "What to rewind?", Items: items})
		}
	case ui.UndoExecute:
		var parts []string

		if act.Mode == "code" || act.Mode == "both" {
			if a.tools.snapshots == nil {
				a.send(ui.CommandResult{Command: "/undo", Error: errors.New("code restore unavailable without git")})

				return
			}

			if err := a.tools.snapshots.RestoreCode(act.Hash); err != nil {
				a.send(ui.CommandResult{Command: "/undo", Error: fmt.Errorf("restoring code: %w", err)})

				return
			}

			parts = append(parts, "code restored")
		}

		if act.Mode == "messages" || act.Mode == "both" {
			history := a.builder.History()
			if act.MessageIndex > 0 && act.MessageIndex < len(history) {
				a.builder.Restore(history[:act.MessageIndex])

				a.tokens.lastInput = 0
				a.tokens.lastOutput = 0
				a.tokens.lastAPIInput = 0
			}

			parts = append(parts, "messages rewound")
		}

		a.send(ui.CommandResult{
			Command: "/undo",
			Message: "Rewound: " + strings.Join(parts, " + "),
			Level:   ui.LevelSuccess,
		})
		a.EmitStatus()
	case ui.RunCommand:
		if a.handleSlash == nil {
			return
		}

		msg, _, forward, err := a.handleSlash(ctx, a, act.Name)
		if err != nil {
			a.send(ui.CommandResult{Error: err})

			return
		}

		if forward && msg != "" {
			if a.stream.active {
				a.stream.pending = append(a.stream.pending, msg)

				return
			}

			a.send(ui.UserMessagesProcessed{Texts: []string{msg}})

			a.processWithTracking(ctx, []string{msg})

			return
		}

		if msg != "" {
			a.send(ui.CommandResult{Message: msg})
		}
	}
}
