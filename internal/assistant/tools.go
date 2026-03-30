package assistant

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/injector"
	"github.com/idelchi/aura/internal/plugins"
	"github.com/idelchi/aura/internal/task"
	"github.com/idelchi/aura/internal/tools"
	"github.com/idelchi/aura/internal/tools/filetime"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/aura/pkg/llm/tool/call"
	"github.com/idelchi/godyl/pkg/env"
)

// preparedCall holds the result of Phase A (pre-flight) for a single tool call.
// Tools that pass all gates land here; denied tools are committed immediately during pre-flight.
type preparedCall struct {
	tc         call.Call       // original tool call (possibly with plugin-modified args)
	tool       tool.Tool       // resolved tool instance
	execCtx    context.Context // context with workdir, SDK context, filetime tracker
	read       []string        // declared read paths (from Paths())
	write      []string        // declared write paths (from Paths())
	hasPaths   bool            // true if Paths() returned non-empty paths without error
	preHookMsg string          // non-blocking message from user pre-hook
}

// execResult holds the output of Phase B (execution) for a single tool call.
type execResult struct {
	output   string
	err      error
	duration time.Duration
}

// IsToolParallel reports whether a tool is safe for concurrent execution,
// checking global parallel toggle, config-level overrides, and the tool's
// ParallelOverride interface. Used by the Batch tool to mirror the main
// pipeline's two-pass dispatch logic.
func (a *Assistant) IsToolParallel(name string) bool {
	if !a.cfg.Features.ToolExecution.ParallelEnabled() {
		return false
	}

	if override := a.cfg.ToolDefs.ParallelOverride(name); override != nil {
		return *override
	}

	t, err := a.rt.AllTools.Get(name)
	if err != nil {
		return true // unknown tool — default parallel
	}

	if po, ok := t.(tool.ParallelOverride); ok {
		return po.Parallel()
	}

	return true
}

// executeTools runs each tool call through a three-phase pipeline:
//
//	Phase A: sequential pre-flight (gates, policy, hooks)
//	Phase B: parallel execution (errgroup for Parallel()==true tools)
//	Phase C: sequential post-processing (builder, stats, injectors)
func (a *Assistant) executeTools(ctx context.Context, toolCalls []call.Call) {
	// Pass 1: register all tool calls upfront as Pending.
	for _, tc := range toolCalls {
		a.builder.AddToolCall(tc.ID, tc.Name, tc.Arguments)
	}

	// ── Phase A: sequential pre-flight ──
	// Each tool runs through all gates. Survivors land in prepared[].
	// Denied tools get immediate CompleteToolCall + AddEphemeralToolResult.
	var prepared []preparedCall

	for _, tc := range toolCalls {
		a.builder.StartToolCall(tc.ID)
		debug.Log("[tool] %s args=%s", tc.Name, truncateArgs(tc.Arguments))

		t, err := a.agent.Tools.Get(tc.Name)
		if err != nil {
			msg := fmt.Sprintf("Error: %v", err)

			if _, allErr := a.rt.AllTools.Get(tc.Name); allErr == nil {
				if _, deferErr := a.tools.deferred.Get(tc.Name); deferErr == nil {
					msg = fmt.Sprintf(
						"Error: tool %q is deferred and not yet loaded. Call LoadTools to load it first.",
						tc.Name,
					)
				} else {
					msg = fmt.Sprintf(
						"Error: tool %q is not available in %q mode. Switch to a mode that enables it.",
						tc.Name,
						a.agent.Mode,
					)
				}
			}

			debug.Log("[tool] %s error: %s", tc.Name, msg)
			a.builder.CompleteToolCall(ctx, tc.ID, "", err)
			a.builder.AddEphemeralToolResult(ctx, tc.Name, tc.ID, msg, 0)

			continue
		}

		// BeforeToolExecution plugin hooks (modify args, block execution).
		state := a.InjectorState()

		{
			beforeResult := a.tools.injectors.RunBeforeTool(ctx, state, tc.Name, tc.Arguments)

			if beforeResult.Block {
				debug.Log("[tool] %s blocked by BeforeToolExecution plugin", tc.Name)

				blockMsg := "Execution blocked by plugin"

				if len(beforeResult.Messages) > 0 && beforeResult.Messages[0].Content != "" {
					blockMsg += ": " + beforeResult.Messages[0].Content
				}

				a.builder.CompleteToolCall(ctx, tc.ID, "", errors.New("blocked by plugin"))
				a.builder.AddEphemeralToolResult(ctx, tc.Name, tc.ID, blockMsg, 0)

				continue
			}

			if beforeResult.Arguments != nil {
				tc.Arguments = beforeResult.Arguments

				if err := t.Schema().ValidateArgs(tc.Arguments); err != nil {
					debug.Log("[tool] %s plugin produced invalid args: %v", tc.Name, err)
					a.builder.CompleteToolCall(ctx, tc.ID, "", err)
					a.builder.AddEphemeralToolResult(ctx, tc.Name, tc.ID,
						fmt.Sprintf("Error: plugin modified arguments are invalid: %v", err), 0)

					continue
				}
			}

			a.injectMessages(beforeResult.Messages)
		}

		// Inject workdir, sdk.Context, and read-before policy for all tool phases (Pre, Execute, Post).
		execCtx := tool.WithWorkDir(ctx, a.effectiveWorkDir())

		execCtx = tool.WithSDKContext(execCtx, plugins.BuildSDKContext(state))
		execCtx = filetime.WithTracker(execCtx, a.tracker)

		// Pre hook.
		if ph, ok := t.(tool.PreHook); ok {
			if err := func() (e error) {
				defer func() {
					if r := recover(); r != nil {
						e = fmt.Errorf("tool panicked in Pre: %v", r)
					}
				}()

				return ph.Pre(execCtx, tc.Arguments)
			}(); err != nil {
				debug.Log("[tool] %s pre-hook error: %v", tc.Name, err)
				a.builder.CompleteToolCall(ctx, tc.ID, "", err)
				a.builder.AddEphemeralToolResult(ctx, tc.Name, tc.ID, fmt.Sprintf("Error: %v", err), 0)

				continue
			}
		}

		// Tool policy check: auto/confirm/deny.
		if !a.resolved.toolPolicy.IsEmpty() {
			switch a.resolved.toolPolicy.Evaluate(tc.Name, tc.Arguments) {
			case config.PolicyDeny:
				pattern := a.resolved.toolPolicy.DenyingPattern(tc.Name, tc.Arguments)
				err := config.DenyError(pattern)
				debug.Log("[tool] %s policy denied: %v", tc.Name, err)
				a.builder.CompleteToolCall(ctx, tc.ID, "", err)
				a.builder.AddEphemeralToolResult(ctx, tc.Name, tc.ID, fmt.Sprintf("Error: %v", err), 0)

				continue

			case config.PolicyConfirm:
				if !a.toggles.auto {
					pattern := deriveToolPattern(tc.Name, tc.Arguments)
					if a.session.approvals[pattern] {
						debug.Log("[tool] %s session-approved: %s", tc.Name, pattern)
					} else {
						action, err := a.confirmTool(ctx, t, tc.Name, tc.Arguments)
						if err != nil || action == ui.ConfirmDeny {
							msg := "tool call denied by user"

							debug.Log("[tool] %s user denied", tc.Name)
							a.builder.CompleteToolCall(ctx, tc.ID, "", errors.New(msg))
							a.builder.AddEphemeralToolResult(ctx, tc.Name, tc.ID, "Error: "+msg, 0)

							continue
						}
					}
				}

			case config.PolicyAuto:
				// Proceed.
			}
		}

		// Guardrail check: validate tool call before execution.
		if blocked, raw, grErr := a.CheckGuardrail(
			ctx,
			"tool_calls",
			tc.Name,
			formatToolCall(tc.Name, tc.Arguments),
		); blocked ||
			grErr != nil {
			msg := formatGuardrailBlock(tc.Name, raw, grErr)
			debug.Log("[guardrail] %s blocked: %s", tc.Name, msg)
			a.builder.CompleteToolCall(ctx, tc.ID, "", errors.New("guardrail blocked"))
			a.builder.AddEphemeralToolResult(ctx, tc.Name, tc.ID, msg, 0)

			continue
		}

		// Path pre-filter: fast-fail if declared paths fall outside sandbox bounds.
		var (
			read, write []string
			pathErr     error
		)

		if pd, ok := t.(tool.PathDeclarer); ok {
			read, write, pathErr = pd.Paths(execCtx, tc.Arguments)
		}

		hasPaths := pathErr == nil && (len(read) > 0 || len(write) > 0)

		if hasPaths && a.resolved.sandbox != nil {
			if err := a.CheckPaths(read, write); err != nil {
				debug.Log("[tool] %s sandbox denied: %v", tc.Name, err)
				a.builder.CompleteToolCall(ctx, tc.ID, "", err)
				a.builder.AddEphemeralToolResult(ctx, tc.Name, tc.ID, fmt.Sprintf("Error: %v", err), 0)

				continue
			}
		}

		// User-configured pre-hooks (can block execution).
		var preHookMsg string

		if preResult := a.resolved.hooks.RunPre(ctx, tc.Name, tc.Arguments, a.effectiveWorkDir()); preResult.Blocked {
			msg := preResult.Message
			if msg == "" {
				msg = "blocked by hook: " + tc.Name
			}

			debug.Log("[tool] %s blocked by pre-hook: %s", tc.Name, msg)
			a.builder.CompleteToolCall(ctx, tc.ID, "", errors.New("blocked by hook"))
			a.builder.AddEphemeralToolResult(ctx, tc.Name, tc.ID, msg, 0)

			continue
		} else if preResult.Message != "" {
			preHookMsg = preResult.Message
		}

		// Tool survived all gates — collect for execution.
		prepared = append(prepared, preparedCall{
			tc:         tc,
			tool:       t,
			execCtx:    execCtx,
			read:       read,
			write:      write,
			hasPaths:   hasPaths,
			preHookMsg: preHookMsg,
		})
	}

	if len(prepared) == 0 {
		return
	}

	// ── Phase B: execution ──
	// Parallel-safe tools run concurrently; non-parallel tools run sequentially after.
	results := make([]execResult, len(prepared))

	if a.cfg.Features.ToolExecution.ParallelEnabled() {
		isParallel := func(pc preparedCall) bool {
			if override := a.cfg.ToolDefs.ParallelOverride(pc.tc.Name); override != nil {
				return *override
			}

			if po, ok := pc.tool.(tool.ParallelOverride); ok {
				return po.Parallel()
			}

			return true
		}

		g, gCtx := errgroup.WithContext(ctx)

		// First pass: dispatch all parallel-safe tools concurrently.
		for i, pc := range prepared {
			if !isParallel(pc) {
				continue
			}

			g.Go(func() error {
				results[i] = a.executeOne(gCtx, pc)

				return nil // errors captured in execResult, never propagated
			})
		}

		g.Wait()

		// Second pass: run non-parallel tools sequentially.
		for i, pc := range prepared {
			if isParallel(pc) {
				continue
			}

			results[i] = a.executeOne(ctx, pc)
		}
	} else {
		// Parallel disabled: run all sequentially (current behavior).
		for i, pc := range prepared {
			results[i] = a.executeOne(ctx, pc)
		}
	}

	// ── Phase C: sequential post-processing ──
	// Iterate in original order. All thread-unsafe operations (builder, stats, Track, injectors) live here.
	for i, pc := range prepared {
		res := results[i]
		tc := pc.tc
		t := pc.tool

		// Setup errors are infrastructure problems the LLM cannot fix — route to user.
		var se *tools.SetupError
		if res.err != nil && errors.As(res.err, &se) {
			debug.Log("[tool] %s setup error in %v: %v", tc.Name, res.duration, se.Err)
			a.send(ui.CommandResult{
				Message: fmt.Sprintf("warning: sandboxed tool setup failed: %v", se.Err),
				Level:   ui.LevelWarn,
			})
			a.builder.CompleteToolCall(ctx, tc.ID, "", res.err)
			a.builder.AddEphemeralToolResult(ctx, tc.Name, tc.ID,
				"Tool unavailable due to configuration error. The user has been notified.", 0)
			a.session.stats.RecordToolError(tc.Name)

			continue
		}

		var result string

		var (
			toolErr     string
			postHookMsg string
		)

		var (
			needsCommit bool
			commitErr   error
		)

		var est int

		output := res.output

		if res.err != nil {
			debug.Log("[tool] %s failed in %v: %v", tc.Name, res.duration, res.err)
			a.session.stats.RecordToolError(tc.Name)

			result = fmt.Sprintf("Error: %v", res.err)
			toolErr = res.err.Error()
			commitErr = res.err
			needsCommit = true
		} else {
			debug.Log("[tool] %s ok in %v result_len=%d", tc.Name, res.duration, len(output))
			a.session.stats.RecordToolCall(tc.Name)

			// Post hook.
			if ph, ok := t.(tool.PostHook); ok {
				func() {
					defer func() {
						if r := recover(); r != nil {
							debug.Log("[tool] %s panicked in Post: %v", tc.Name, r)
						}
					}()

					ph.Post(pc.execCtx, tc.Arguments)
				}()
			}

			// User-configured post-hooks.
			cwd := a.effectiveWorkDir()
			if postResult := a.resolved.hooks.RunPost(
				ctx,
				tc.Name,
				tc.Arguments,
				output,
				cwd,
			); postResult.Message != "" {
				postHookMsg = postResult.Message
			}

			// LSP diagnostics on modified files.
			wantsLSP := false

			if la, ok := t.(tool.LSPAware); ok {
				wantsLSP = la.WantsLSP()
			}

			if a.tools.lsp != nil && wantsLSP && pc.hasPaths && len(pc.write) > 0 {
				a.send(ui.SpinnerMessage{Text: "LSP diagnostics…"})

				for _, p := range pc.write {
					a.tools.lsp.NotifyChange(ctx, p)
				}

				var diagParts []string

				for _, p := range pc.write {
					if d := a.tools.lsp.FormatDiagnostics(p); d != "" {
						diagParts = append(diagParts, d)
					}
				}

				if len(diagParts) > 0 {
					output += "\n\n[LSP diagnostics]:\n" + strings.Join(diagParts, "\n")
				}
			}

			// Guard: reject oversized results before they enter conversation history.
			if estVal, rejected, msg := a.CheckResult(ctx, output); rejected {
				debug.Log("[tool] %s result rejected: too large (%d tokens)", tc.Name, estVal)

				result = msg

				a.builder.CompleteToolCall(ctx, tc.ID, msg, nil)
				a.builder.AddToolResult(ctx, tc.Name, tc.ID, msg, 0)
			} else {
				result = output
				est = estVal
				needsCommit = true
			}
		}

		// Track for injector state and run AfterToolExecution injectors.
		a.Track(tc.Name, tc.Arguments, result, toolErr, res.duration)

		state := a.InjectorState()
		injections := a.tools.injectors.RunAfterTool(ctx, state)

		// Apply Output modification from AfterToolExecution hooks.
		if needsCommit {
			for _, inj := range injections {
				if inj.Output != nil {
					debug.Log("[tool] %s output modified by plugin %s (old_len=%d new_len=%d)",
						tc.Name, inj.Name, len(result), len(*inj.Output))

					result = *inj.Output
				}
			}

			if result != output {
				est = a.estimator.Estimate(ctx, result)
			}

			a.builder.CompleteToolCall(ctx, tc.ID, result, commitErr)
			a.builder.AddToolResult(ctx, tc.Name, tc.ID, result, est)

			if postHookMsg != "" {
				a.builder.AddEphemeralToolResult(ctx, tc.Name, tc.ID+"-hook-post", postHookMsg, 0)
			}

			if preHookMsg := pc.preHookMsg; preHookMsg != "" {
				a.builder.AddEphemeralToolResult(ctx, tc.Name, tc.ID+"-hook-pre", preHookMsg, 0)
			}
		}

		// Filter out Output-only injections.
		var messageInjections []injector.Injection

		for _, inj := range injections {
			if inj.Content != "" || inj.DisplayOnly {
				messageInjections = append(messageInjections, inj.Injection)
			}
		}

		a.injectMessages(messageInjections)
	}
}

// executeOne runs a single tool (sandbox or direct) and returns the result.
// Only touches thread-safe components — safe for concurrent use in Phase B goroutines.
func (a *Assistant) executeOne(ctx context.Context, pc preparedCall) execResult {
	a.send(ui.SpinnerMessage{Text: pc.tc.Name + " running…"})

	streamCtx := tool.WithStreamCallback(pc.execCtx, func(line string) {
		a.send(ui.ToolOutputDelta{ToolName: pc.tc.Name, Line: line})
	})

	start := time.Now()

	var (
		output string
		err    error
	)

	sandboxable := true

	if so, ok := pc.tool.(tool.SandboxOverride); ok {
		sandboxable = so.Sandboxable()
	}

	if a.toggles.sandbox && sandboxable {
		output, err = a.executeSandboxed(streamCtx, pc.tc.Name, pc.tc.Arguments)
	} else {
		output, err = func() (s string, e error) {
			defer func() {
				if r := recover(); r != nil {
					e = fmt.Errorf("tool panicked: %v", r)
				}
			}()

			return pc.tool.Execute(streamCtx, pc.tc.Arguments)
		}()
	}

	return execResult{output: output, err: err, duration: time.Since(start)}
}

// executeSandboxed runs a tool via self-re-exec with Landlock sandboxing.
func (a *Assistant) executeSandboxed(ctx context.Context, toolName string, args map[string]any) (string, error) {
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return "", fmt.Errorf("marshaling arguments: %w", err)
	}

	restrictions := a.cfg.EffectiveRestrictions()

	// Root flags must come before the subcommand (Local: true).
	var cmdArgs []string

	for _, home := range a.configOpts.Homes {
		if home != a.configOpts.GlobalHome && home != "" {
			cmdArgs = append(cmdArgs, "--config", home)
		}
	}

	if a.rt.UnsafePlugins {
		cmdArgs = append(cmdArgs, "--unsafe-plugins")
	}

	if !a.rt.WithPlugins {
		cmdArgs = append(cmdArgs, "--without-plugins")
	}

	// Subcommand + subcommand-specific flags.
	cmdArgs = append(cmdArgs, "tools", "--json", "--raw", "-H")

	for _, p := range restrictions.ReadOnly {
		cmdArgs = append(cmdArgs, "--ro-paths", p)
	}

	for _, p := range restrictions.ReadWrite {
		cmdArgs = append(cmdArgs, "--rw-paths", p)
	}

	cmdArgs = append(cmdArgs, toolName, string(argsJSON))

	// Resolve the executable to an absolute path so cmd.Dir doesn't break
	// relative os.Args[0] references (e.g. "./aura" when Dir != launch dir).
	exe, err := os.Executable()
	if err != nil {
		exe = os.Args[0]
	}

	cmd := exec.CommandContext(ctx, exe, cmdArgs...)

	cmd.Dir = tool.WorkDirFromContext(ctx)

	if taskEnv := task.EnvFromContext(ctx); len(taskEnv) > 0 {
		merged := taskEnv.MergedWith(env.FromEnv())

		cmd.Env = merged.AsSlice()
	}

	// Pipe sdk.Context to child via stdin (bypasses ARG_MAX).
	sdkCtx := plugins.BuildSDKContext(a.InjectorState())

	ctxJSON, err := json.Marshal(sdkCtx)
	if err != nil {
		return "", fmt.Errorf("sandbox context marshal: %w", err)
	}

	cmd.Stdin = bytes.NewReader(ctxJSON)

	// Pipe stdout (JSON result) and stderr (streaming + errors) separately
	// so we can stream incremental output from the child process.
	var stdoutBuf bytes.Buffer

	cmd.Stdout = &stdoutBuf

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("creating stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("starting sandboxed tool: %w", err)
	}

	// Read stderr in a goroutine: lines with \x00STREAM: prefix are streaming
	// output; everything else is collected for error reporting.
	var stderrBuf bytes.Buffer

	cb := tool.StreamCallbackFromContext(ctx)

	var wg sync.WaitGroup

	wg.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				debug.Log("[sandbox] stderr reader panicked: %v", r)
			}
		}()

		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			line := scanner.Text()
			if after, ok := strings.CutPrefix(line, "\x00STREAM:"); ok {
				if cb != nil {
					cb(after)
				}
			} else {
				stderrBuf.WriteString(line)
				stderrBuf.WriteByte('\n')
			}
		}
	})

	// Drain pipe fully before Wait() — otherwise Wait() may close the pipe
	// while the goroutine is still reading.
	wg.Wait()

	err = cmd.Wait()

	output := stdoutBuf.Bytes()

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// Safety net: try parsing stdout before falling back to stderr.
			// After the child fix, setup errors exit 0, so ExitError should
			// only happen for truly unexpected crashes.
			var tr tools.ToolResult
			if jsonErr := json.Unmarshal(output, &tr); jsonErr == nil && tr.Setup {
				return "", &tools.SetupError{Err: errors.New(tr.Error)}
			}

			return "", fmt.Errorf("%w\nStderr: %s", err, stderrBuf.String())
		}

		return "", err
	}

	var tr tools.ToolResult
	if err := json.Unmarshal(output, &tr); err != nil {
		return string(output), nil // Return raw if not JSON
	}

	if tr.Error != "" {
		if tr.Setup {
			return "", &tools.SetupError{Err: errors.New(tr.Error)}
		}

		return "", errors.New(tr.Error)
	}

	return tr.Stdout, nil
}

// truncateArgs formats tool arguments as a string, truncated to 200 characters.
func truncateArgs(args map[string]any) string {
	s := fmt.Sprintf("%v", args)
	if len(s) > 200 {
		return s[:200] + "..."
	}

	return s
}

// CheckResult estimates the tool result and rejects it if it exceeds the configured limit.
// In "tokens" mode, rejects when the result alone exceeds Result.MaxTokens.
// In "percentage" mode, rejects when adding the result would push context usage above Result.MaxPercentage.
// Returns the token estimate (always computed), whether the result was rejected, and the rejection message.
func (a *Assistant) CheckResult(ctx context.Context, output string) (est int, rejected bool, msg string) {
	cfg := a.cfg.Features.ToolExecution

	est = a.estimator.Estimate(ctx, output)

	switch cfg.Mode {
	case "percentage":
		contextLen := a.ContextLength()
		if contextLen == 0 {
			return est, false, ""
		}

		current := a.Tokens()
		projected := contextLen.PercentUsed(current + est)

		remaining := int(contextLen) - current
		remainingPct := cfg.Result.MaxPercentage - contextLen.PercentUsed(current)

		if projected > cfg.Result.MaxPercentage {
			return est, true, fmt.Sprintf(
				"Error: Tool result too large (%d tokens). Adding it would push context to %.0f%% (limit %.0f%%). Remaining budget: %d tokens (%.0f%%). Current context: %.0f%%.",
				est,
				projected,
				cfg.Result.MaxPercentage,
				remaining,
				remainingPct,
				contextLen.PercentUsed(current),
			)
		}

	default: // "tokens"
		if cfg.Result.MaxTokens <= 0 {
			return est, false, ""
		}

		if est > cfg.Result.MaxTokens {
			return est, true, fmt.Sprintf(cfg.RejectionMessage, est, cfg.Result.MaxTokens)
		}
	}

	return est, false, ""
}

// CheckInput estimates user input and rejects it if adding it would push context
// usage above the configured percentage.
// Returns the token estimate (always computed) and a rejection message ("" if accepted).
func (a *Assistant) CheckInput(ctx context.Context, text string) (est int, msg string) {
	est = a.estimator.Estimate(ctx, text)

	maxPct := a.cfg.Features.ToolExecution.UserInputMaxPercentage
	if maxPct <= 0 {
		return est, ""
	}

	contextLen := a.ContextLength()
	if contextLen == 0 {
		return est, ""
	}

	current := a.Tokens()
	projected := contextLen.PercentUsed(current + est)

	if projected <= maxPct {
		return est, ""
	}

	// Budget = tokens available before hitting the percentage cap
	budget := int(float64(contextLen)*maxPct/100.0) - current
	if budget <= 0 {
		return est, fmt.Sprintf(
			"Context is %.0f%% full (limit: %.0f%%). Run /compact to free space.",
			contextLen.PercentUsed(current), maxPct,
		)
	}

	return est, fmt.Sprintf(
		"Message too large (%d estimated tokens, budget %d). Try a shorter message.",
		est, budget,
	)
}

// confirmTool sends a confirmation prompt to the UI and blocks until the user responds.
func (a *Assistant) confirmTool(
	ctx context.Context,
	t tool.Tool,
	toolName string,
	args map[string]any,
) (ui.ConfirmAction, error) {
	respCh := make(chan ui.ConfirmAction, 1)

	pattern := deriveToolPattern(toolName, args)

	// Build detail string for display.
	var detail string

	if toolName == "Bash" {
		if cmd, ok := args["command"].(string); ok {
			detail = cmd
		}
	} else {
		for _, key := range []string{"file_path", "path", "pattern", "command"} {
			if v, ok := args[key].(string); ok && v != "" {
				detail = v

				break
			}
		}
	}

	// Generate diff preview if the tool supports it (best-effort).
	var diffPreview string

	if p, ok := t.(tool.Previewer); ok {
		previewCtx := tool.WithWorkDir(ctx, a.effectiveWorkDir())
		if d, err := p.Preview(previewCtx, args); err == nil {
			diffPreview = d
		} else {
			debug.Log("[tool] %s preview error (falling back to detail-only): %v", toolName, err)
		}
	}

	a.send(ui.ToolConfirmRequired{
		ToolName:    toolName,
		Description: t.Description(),
		Detail:      detail,
		DiffPreview: diffPreview,
		Pattern:     pattern,
		Response:    respCh,
	})

	select {
	case action := <-respCh:
		switch action {
		case ui.ConfirmAllowSession:
			if pattern != "" {
				a.session.approvals[pattern] = true
				debug.Log("[tool] session approval saved: %s", pattern)
			}
		case ui.ConfirmAllowPatternProject:
			if pattern != "" {
				if err := config.SaveToolApproval(a.configOpts.WriteHome, pattern); err != nil {
					debug.Log("[tool] failed to save project approval: %v", err)
					a.send(
						ui.CommandResult{
							Message: fmt.Sprintf("warning: could not save tool approval: %v", err),
							Level:   ui.LevelWarn,
						},
					)
				} else {
					debug.Log("[tool] project approval saved: %s", pattern)
				}
			}
		case ui.ConfirmAllowPatternGlobal:
			if pattern != "" {
				if a.configOpts.GlobalHome == "" {
					debug.Log("[tool] global approval skipped: no global home configured")
					a.send(
						ui.CommandResult{
							Message: "warning: global approval not saved (no --home configured)",
							Level:   ui.LevelWarn,
						},
					)
				} else if err := config.SaveToolApproval(a.configOpts.GlobalHome, pattern); err != nil {
					debug.Log("[tool] failed to save global approval: %v", err)
					a.send(
						ui.CommandResult{
							Message: fmt.Sprintf("warning: could not save tool approval: %v", err),
							Level:   ui.LevelWarn,
						},
					)
				} else {
					debug.Log("[tool] global approval saved: %s", pattern)
				}
			}
		}

		return action, nil
	case <-ctx.Done():
		return ui.ConfirmDeny, ctx.Err()
	}
}

// deriveToolPattern builds the persistence pattern for a tool call.
// For Bash: "Bash:git commit*" (uses bash command pattern derivation).
// For file tools with a path arg: "Write:/tmp/*" (directory-scoped).
// For other tools: just the tool name (e.g. "Patch").
func deriveToolPattern(toolName string, args map[string]any) string {
	if toolName == "Bash" {
		if cmd, ok := args["command"].(string); ok {
			return "Bash:" + deriveBashPattern(cmd)
		}
	}

	if detail := config.ExtractToolDetail(toolName, args); detail != "" {
		return toolName + ":" + derivePathPattern(detail)
	}

	return toolName
}

// derivePathPattern converts a file path into a directory-scoped wildcard pattern.
// "/tmp/foo.txt" → "/tmp/*", "/etc/hosts" → "/etc/*", "/file.txt" → "/*".
func derivePathPattern(path string) string {
	dir := filepath.Dir(path)
	if dir == "." {
		return "*"
	}

	if dir == "/" {
		return "/*"
	}

	return dir + "/*"
}

// deriveBashPattern extracts a command prefix pattern for "allow always" suggestions.
// "git commit -m 'fix bug'" → "git commit*"
// "npm install express" → "npm install*"
// "rm -rf node_modules" → "rm*"
// multiWord lists commands whose subcommand matters for pattern matching.
var multiWord = map[string]bool{
	"git": true, "npm": true, "npx": true, "yarn": true, "pnpm": true,
	"docker": true, "kubectl": true, "go": true, "cargo": true,
	"pip": true, "gem": true, "mix": true, "make": true,
}

func deriveBashPattern(command string) string {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return ""
	}

	// For known multi-word commands, use first two words.
	if len(parts) >= 2 && multiWord[parts[0]] {
		return parts[0] + " " + parts[1] + "*"
	}

	return parts[0] + "*"
}

// ExecuteSubTool runs a single tool call through a minimal pipeline.
// Used by the Batch tool for concurrent sub-calls. Runs security-critical steps
// (plugins, policy, guardrails, hooks, sandbox) but skips conversation-history steps
// (builder, tracking, LSP, stats, streaming).
func (a *Assistant) ExecuteSubTool(ctx context.Context, toolName string, args map[string]any) (string, error) {
	t, err := a.agent.Tools.Get(toolName)
	if err != nil {
		return "", fmt.Errorf("tool %q not found", toolName)
	}

	// BeforeToolExecution plugins — plugin contracts are tool-specific.
	state := a.InjectorState()
	beforeResult := a.tools.injectors.RunBeforeTool(ctx, state, toolName, args)

	if beforeResult.Block {
		return "", errors.New("blocked by plugin")
	}

	if beforeResult.Arguments != nil {
		args = beforeResult.Arguments

		if err := t.Schema().ValidateArgs(args); err != nil {
			return "", fmt.Errorf("plugin modified arguments are invalid: %w", err)
		}
	}

	// Tool policy — deny/confirm must apply per sub-call (security).
	if !a.resolved.toolPolicy.IsEmpty() {
		switch a.resolved.toolPolicy.Evaluate(toolName, args) {
		case config.PolicyDeny:
			return "", config.DenyError(a.resolved.toolPolicy.DenyingPattern(toolName, args))
		case config.PolicyConfirm:
			if !a.toggles.auto {
				return "", fmt.Errorf(
					"tool %q requires user confirmation and cannot be batched — call it directly",
					toolName,
				)
			}
		}
	}

	// Guardrail check: validate tool call before execution.
	if blocked, raw, grErr := a.CheckGuardrail(
		ctx,
		"tool_calls",
		toolName,
		formatToolCall(toolName, args),
	); blocked ||
		grErr != nil {
		return "", fmt.Errorf("guardrail blocked: %s", formatGuardrailBlock(toolName, raw, grErr))
	}

	// Context with workdir, SDK context, and filetime tracker.
	execCtx := tool.WithWorkDir(ctx, a.effectiveWorkDir())

	execCtx = tool.WithSDKContext(execCtx, plugins.BuildSDKContext(state))
	execCtx = filetime.WithTracker(execCtx, a.tracker)

	// Pre hook — panic recovery required since sub-calls run in goroutines.
	if ph, ok := t.(tool.PreHook); ok {
		if err := func() (e error) {
			defer func() {
				if r := recover(); r != nil {
					e = fmt.Errorf("tool panicked in Pre: %v", r)
				}
			}()

			return ph.Pre(execCtx, args)
		}(); err != nil {
			return "", fmt.Errorf("pre-hook: %w", err)
		}
	}

	// Sandbox path check.
	var (
		read, write []string
		pathErr     error
	)

	if pd, ok := t.(tool.PathDeclarer); ok {
		read, write, pathErr = pd.Paths(execCtx, args)
	}

	if pathErr == nil && (len(read) > 0 || len(write) > 0) {
		if a.resolved.sandbox != nil {
			if err := a.CheckPaths(read, write); err != nil {
				return "", fmt.Errorf("sandbox denied: %w", err)
			}
		}
	}

	// User pre-hooks (can block).
	cwd := a.effectiveWorkDir()

	if preResult := a.resolved.hooks.RunPre(ctx, toolName, args, cwd); preResult.Blocked {
		msg := preResult.Message
		if msg == "" {
			msg = "blocked by hook: " + toolName
		}

		return "", fmt.Errorf("%s", msg)
	}

	// Execute (sandboxed or direct) — panic recovery on direct path.
	var output string

	sandboxable := true

	if so, ok := t.(tool.SandboxOverride); ok {
		sandboxable = so.Sandboxable()
	}

	if a.toggles.sandbox && sandboxable {
		output, err = a.executeSandboxed(ctx, toolName, args)
	} else {
		output, err = func() (s string, e error) {
			defer func() {
				if r := recover(); r != nil {
					e = fmt.Errorf("tool panicked: %v", r)
				}
			}()

			return t.Execute(execCtx, args)
		}()
	}

	if err != nil {
		return "", err
	}

	// Post hook — panic recovery.
	if ph, ok := t.(tool.PostHook); ok {
		func() {
			defer func() {
				if r := recover(); r != nil {
					debug.Log("[batch-sub] %s panicked in Post: %v", toolName, r)
				}
			}()

			ph.Post(execCtx, args)
		}()
	}

	// User post-hooks (message appended to output).
	if postResult := a.resolved.hooks.RunPost(ctx, toolName, args, output, cwd); postResult.Message != "" {
		output += "\n\n[hook] " + postResult.Message
	}

	return output, nil
}

// CheckPaths validates that all paths a tool will access are allowed by the sandbox.
func (a *Assistant) CheckPaths(read, write []string) error {
	for _, p := range read {
		if !a.resolved.sandbox.CanRead(p) {
			return fmt.Errorf("sandbox: read access denied for %q", p)
		}
	}

	for _, p := range write {
		if !a.resolved.sandbox.CanWrite(p) {
			return fmt.Errorf("sandbox: write access denied for %q", p)
		}
	}

	return nil
}
