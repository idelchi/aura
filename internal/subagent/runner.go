// Package subagent provides a mini tool loop for spawning one-shot subagents
// with isolated conversation context and a restricted tool set.
package subagent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/conversation"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/hooks"
	"github.com/idelchi/aura/internal/lsp"
	"github.com/idelchi/aura/internal/tools"
	"github.com/idelchi/aura/internal/tools/filetime"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/stream"
	"github.com/idelchi/aura/pkg/llm/thinking"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/aura/pkg/llm/tool/call"
	"github.com/idelchi/aura/pkg/llm/usage"
	"github.com/idelchi/aura/pkg/providers"
)

// PathChecker validates that declared tool paths are allowed.
// Return non-nil error to reject the tool call.
type PathChecker func(read, write []string) error

// ResultGuardFunc checks whether a tool result is acceptable (e.g. size limits).
// Return non-nil error to reject the result.
type ResultGuardFunc func(ctx context.Context, toolName, result string) error

// Result is the outcome of a subagent run.
type Result struct {
	// Text is the final assistant response (from the turn with no tool calls).
	Text string
	// ToolCalls is the total number of tool calls executed.
	ToolCalls int
	// Tools maps tool name → call count for summary display.
	Tools map[string]int
	// Usage is aggregated token usage across all LLM calls.
	Usage usage.Usage
}

// Runner executes a mini tool loop: system prompt → user prompt → LLM ↔ tools → text response.
type Runner struct {
	// Provider is the LLM backend to call.
	Provider providers.Provider
	// Tools is the set of tools available to the subagent.
	Tools tool.Tools
	// Prompt is the system prompt for the subagent.
	Prompt string
	// Model is the resolved model to use for requests.
	Model model.Model
	// Events receives conversation events (can be a no-op sink).
	Events conversation.EventSink
	// MaxSteps caps the total number of LLM round-trips to prevent runaway loops.
	MaxSteps int
	// Think configures reasoning mode for requests (nil = off).
	Think *thinking.Value
	// ContextLength overrides the model's default context window. Zero = use Model.ContextLength.
	ContextLength int
	// ExecuteOverride replaces t.Execute() for sandboxable tools (e.g. Landlock re-exec).
	// If nil, all tools execute directly via t.Execute().
	ExecuteOverride func(ctx context.Context, toolName string, args map[string]any) (string, error)
	// PathChecker fast-fails tool calls whose declared paths fall outside sandbox bounds.
	// If nil, no path checking is performed.
	PathChecker PathChecker
	// ResultGuard optionally validates tool results (e.g. size limits). Nil = no validation.
	ResultGuard ResultGuardFunc
	// HooksRunner runs user-configured shell hooks before/after tool calls. Zero value = no hooks.
	HooksRunner hooks.Runner
	// LSPManager collects diagnostics after tool execution. Nil = no LSP diagnostics.
	LSPManager *lsp.Manager
	// CWD is the working directory passed to hook scripts. Empty = hooks receive empty cwd.
	CWD string
	// Estimate is the token estimation function.
	Estimate func(string) int
	// Features is the child's resolved feature set (global → agent → mode).
	// Used by downstream behavior (bash truncation, etc.).
	Features config.Features
	// ReadBeforePolicy is the resolved read-before enforcement policy for this subagent.
	// Created from the child's config, with parent's runtime override taking precedence.
	ReadBeforePolicy tool.ReadBeforePolicy
}

// Run executes the subagent loop: sends the prompt, iterates tool calls, and returns
// the final text response when the LLM produces a turn with no tool calls.
func (r *Runner) Run(ctx context.Context, prompt string) (Result, error) {
	debug.Log("[subagent] starting: model=%s tools=%d maxSteps=%d", r.Model.Name, len(r.Tools), r.MaxSteps)

	var systemTokens int

	if r.Estimate != nil {
		systemTokens = r.Estimate(r.Prompt)
	}

	var builderEstimate func(context.Context, string) int

	if r.Estimate != nil {
		est := r.Estimate

		builderEstimate = func(_ context.Context, s string) int { return est(s) }
	}

	// Create a fresh Tracker for this subagent session — isolated from parent and siblings.
	tracker := filetime.NewTracker(r.ReadBeforePolicy)

	ctx = filetime.WithTracker(ctx, tracker)

	builder := conversation.NewBuilder(r.Events, r.Prompt, systemTokens, builderEstimate)
	builder.AddUserMessage(ctx, prompt, 0)

	result := Result{
		Tools: make(map[string]int),
	}

	contextLength := r.ContextLength
	if contextLength == 0 {
		contextLength = int(r.Model.ContextLength)
	}

	for step := range r.MaxSteps {
		req := request.Request{
			Model:         r.Model,
			Think:         r.Think,
			Messages:      builder.History().ForLLM(),
			ContextLength: contextLength,
			Tools:         r.Tools.Schemas(),
		}

		resp, u, err := r.Provider.Chat(ctx, req, stream.Func(nil))
		if err != nil {
			result.Text = lastAssistantContent(builder.History())

			return result, fmt.Errorf("step %d: %w", step, err)
		}

		result.Usage.Add(u)
		builder.AddAssistantMessage(resp)

		if len(resp.Calls) == 0 {
			result.Text = resp.Content
			debug.Log(
				"[subagent] complete: steps=%d totalCalls=%d responseLen=%d",
				step+1,
				result.ToolCalls,
				len(result.Text),
			)

			return result, nil
		}

		debug.Log("[subagent] step %d: %d tool calls", step+1, len(resp.Calls))

		for _, tc := range resp.Calls {
			r.executeToolCall(ctx, builder, tc, &result)
		}
	}

	result.Text = lastAssistantContent(builder.History())

	note := fmt.Sprintf("[budget exhausted after %d steps]", r.MaxSteps)

	if result.Text != "" {
		result.Text += "\n\n" + note
	} else {
		result.Text = note
	}

	return result, nil
}

// lastAssistantContent walks backward through history and returns the Content
// of the first assistant message with non-empty Content. Tool-call-only assistant
// messages (empty Content) are skipped. Returns "" if no qualifying message exists.
func lastAssistantContent(history message.Messages) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].IsAssistant() && history[i].Content != "" {
			return history[i].Content
		}
	}

	return ""
}

// executeToolCall runs a single tool call through the subagent pipeline.
//
// Intentional divergences from the assistant's executeTools() (tools.go):
//   - No UI events (SpinnerMessage, CompleteToolCall) — runner is headless
//   - No tool policy (confirm/deny/auto) — subagents are non-interactive
//   - No guardrail checks — subagents are trusted internal calls
//   - No injectors (AfterToolExecution) — runner has no injector registry
//   - No session stats (RecordToolCall/RecordToolError) — runner uses simple counters on Result
//   - Enforcement controlled by ReadBeforePolicy on context — defaults apply
//   - No mode-aware "tool not found" — runner is config-agnostic
func (r *Runner) executeToolCall(ctx context.Context, builder *conversation.Builder, tc call.Call, result *Result) {
	t, err := r.Tools.Get(tc.Name)
	if err != nil {
		builder.AddToolResult(ctx, tc.Name, tc.ID, fmt.Sprintf("Error: tool %q not found", tc.Name), 0)

		return
	}

	// Inject workdir into context so tools resolve paths against the correct directory.
	ctx = tool.WithWorkDir(ctx, r.CWD)

	// 1. Pre hook (filetime tracking, etc.)
	if ph, ok := t.(tool.PreHook); ok {
		if err := func() (e error) {
			defer func() {
				if r := recover(); r != nil {
					e = fmt.Errorf("tool panicked in Pre: %v", r)
				}
			}()

			return ph.Pre(ctx, tc.Arguments)
		}(); err != nil {
			builder.AddToolResult(ctx, tc.Name, tc.ID, fmt.Sprintf("Error: %v", err), 0)

			return
		}
	}

	// 2. Resolve paths once — reused by PathChecker (step 2b) and LSP (step 5d).
	var (
		read, write []string
		pathErr     error
	)

	if pd, ok := t.(tool.PathDeclarer); ok {
		read, write, pathErr = pd.Paths(ctx, tc.Arguments)
	}

	hasPaths := pathErr == nil && (len(read) > 0 || len(write) > 0)

	// 2b. Path pre-check — fast-fail if declared paths fall outside sandbox bounds.
	if r.PathChecker != nil && hasPaths {
		if err := r.PathChecker(read, write); err != nil {
			builder.AddToolResult(ctx, tc.Name, tc.ID, fmt.Sprintf("Error: %v", err), 0)

			return
		}
	}

	// 3. User-configured pre-hooks (can block execution)
	var preHookMsg string

	if preResult := r.HooksRunner.RunPre(ctx, tc.Name, tc.Arguments, r.CWD); preResult.Blocked {
		msg := preResult.Message
		if msg == "" {
			msg = "blocked by hook: " + tc.Name
		}

		builder.AddToolResult(ctx, tc.Name, tc.ID, msg, 0)

		return
	} else if preResult.Message != "" {
		preHookMsg = preResult.Message
	}

	// 4. Execute — Landlock re-exec for sandboxable tools, direct otherwise
	var (
		output  string
		execErr error
	)

	sandboxable := true

	if so, ok := t.(tool.SandboxOverride); ok {
		sandboxable = so.Sandboxable()
	}

	if r.ExecuteOverride != nil && sandboxable {
		output, execErr = r.ExecuteOverride(ctx, tc.Name, tc.Arguments)
	} else {
		output, execErr = func() (s string, e error) {
			defer func() {
				if rv := recover(); rv != nil {
					e = fmt.Errorf("tool panicked: %v", rv)
				}
			}()

			return t.Execute(ctx, tc.Arguments)
		}()
	}

	if execErr != nil {
		var se *tools.SetupError
		if errors.As(execErr, &se) {
			debug.Log("[subagent] %s setup error: %v", tc.Name, se.Err)
			builder.AddEphemeralToolResult(ctx, tc.Name, tc.ID,
				"Tool unavailable due to configuration error.", 0)

			result.ToolCalls++

			result.Tools[tc.Name]++

			return
		}

		builder.AddToolResult(ctx, tc.Name, tc.ID, fmt.Sprintf("Error: %v", execErr), 0)

		result.ToolCalls++

		result.Tools[tc.Name]++

		return
	}

	// 5. Post hook
	if ph, ok := t.(tool.PostHook); ok {
		func() {
			defer func() {
				if rv := recover(); rv != nil {
					debug.Log("[subagent] %s panicked in Post: %v", tc.Name, rv)
				}
			}()

			ph.Post(ctx, tc.Arguments)
		}()
	}

	// 5b. User-configured post-hooks (append messages to output)
	if postResult := r.HooksRunner.RunPost(ctx, tc.Name, tc.Arguments, output, r.CWD); postResult.Message != "" {
		output = output + "\n\n" + postResult.Message
	}

	// 5c. Prepend pre-hook message (non-blocking warnings from pre-hooks)
	if preHookMsg != "" {
		output = preHookMsg + "\n\n" + output
	}

	// 5d. LSP diagnostics on modified files (after hooks/formatters have run).
	wantsLSP := false

	if la, ok := t.(tool.LSPAware); ok {
		wantsLSP = la.WantsLSP()
	}

	if r.LSPManager != nil && wantsLSP && hasPaths && len(write) > 0 {
		for _, p := range write {
			r.LSPManager.NotifyChange(ctx, p)
		}

		var diagParts []string

		for _, p := range write {
			if d := r.LSPManager.FormatDiagnostics(p); d != "" {
				diagParts = append(diagParts, d)
			}
		}

		if len(diagParts) > 0 {
			output += "\n\n[LSP diagnostics]:\n" + strings.Join(diagParts, "\n")
		}
	}

	// 6. Result guard
	if r.ResultGuard != nil {
		if guardErr := r.ResultGuard(ctx, tc.Name, output); guardErr != nil {
			builder.AddToolResult(ctx, tc.Name, tc.ID, fmt.Sprintf("Error: %v", guardErr), 0)

			result.ToolCalls++

			result.Tools[tc.Name]++

			return
		}
	}

	builder.AddToolResult(ctx, tc.Name, tc.ID, output, 0)

	result.ToolCalls++

	result.Tools[tc.Name]++
}
