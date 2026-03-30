package assistant

import (
	"context"
	"errors"
	"fmt"
	"strings"

	humanize "github.com/dustin/go-humanize"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/injector"
	"github.com/idelchi/aura/internal/providers"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/stream"
)

// ErrCompactionConfig indicates a configuration error that will never succeed on retry.
var ErrCompactionConfig = errors.New("compaction misconfigured")

// errNoSummarizationAgent indicates no agent/prompt is configured for summarization.
// This is not a misconfiguration — it means "skip LLM summarization, keep mechanical pruning".
var errNoSummarizationAgent = errors.New("no summarization agent configured")

// ErrMaxSteps indicates the assistant exceeded its configured step limit.
var ErrMaxSteps = errors.New("maximum steps exceeded")

// ErrTokenBudget indicates the assistant exceeded its cumulative token budget.
var ErrTokenBudget = errors.New("token budget exceeded")

// ErrCompactionExhausted indicates all compaction recovery attempts failed.
var ErrCompactionExhausted = errors.New("compaction recovery exhausted")

// maxCompactionAttempts caps the number of compaction retries before giving up.
// Safety limit — not configurable. Prevents burning LLM calls on misconfigured providers.
const maxCompactionAttempts = 5

// noopStream is a no-op stream handler used by sub-agent LLM calls (compaction, title, thinking).
var noopStream = stream.Func(func(_, _ string, _ bool) error { return nil })

// Tokens returns the best available token count for context usage checks.
// Prefers the actual input tokens from the last API call; falls back to a rough
// client-side estimate when no API call has been made yet.
func (a *Assistant) Tokens() int {
	if a.tokens.lastInput > 0 {
		return a.tokens.lastInput
	}

	return a.EstimateTokens(context.Background())
}

// HasCompactor returns true if a compaction agent or prompt is configured.
// When false, compaction skips LLM summarization and only performs mechanical pruning.
func (a *Assistant) HasCompactor() bool {
	cfg := a.cfg.Features.Compaction

	return cfg.Prompt != "" || cfg.Agent != ""
}

// ShouldCompact checks if context usage exceeds the compaction threshold.
// When MaxTokens is set, uses absolute token comparison (no context length needed).
// Otherwise falls back to percentage of context window.
func (a *Assistant) ShouldCompact() bool {
	cfg := a.cfg.Features.Compaction
	tokens := a.Tokens()

	// Absolute threshold takes priority — no context length needed.
	if cfg.MaxTokens > 0 {
		result := tokens >= cfg.MaxTokens

		debug.Log("[compact] ShouldCompact: tokens=%d maxTokens=%d result=%v lastInputTokens=%d",
			tokens, cfg.MaxTokens, result, a.tokens.lastInput)

		return result
	}

	// Percentage threshold — requires context length.
	contextLen := a.ContextLength()
	if contextLen == 0 {
		return false
	}

	pct := contextLen.PercentUsed(tokens)
	threshold := cfg.Threshold
	result := pct >= threshold

	debug.Log("[compact] ShouldCompact: tokens=%d contextLen=%d pct=%.0f threshold=%.0f result=%v lastInputTokens=%d",
		tokens, int(contextLen), pct, threshold, result, a.tokens.lastInput)

	return result
}

// ShouldTrim checks if context usage exceeds the synthetic trim threshold.
// When TrimMaxTokens is set, uses absolute token comparison (no context length needed).
// Otherwise falls back to percentage of context window.
func (a *Assistant) ShouldTrim() bool {
	cfg := a.cfg.Features.Compaction
	tokens := a.Tokens()

	// Absolute threshold takes priority — no context length needed.
	if cfg.TrimMaxTokens > 0 {
		result := tokens >= cfg.TrimMaxTokens

		debug.Log("[compact] ShouldTrim: tokens=%d trimMaxTokens=%d result=%v",
			tokens, cfg.TrimMaxTokens, result)

		return result
	}

	// Percentage threshold — requires context length.
	contextLen := a.ContextLength()
	if contextLen == 0 {
		return false
	}

	pct := contextLen.PercentUsed(tokens)
	threshold := cfg.TrimThreshold
	result := pct >= threshold

	debug.Log("[compact] ShouldTrim: tokens=%d contextLen=%d pct=%.0f threshold=%.0f result=%v",
		tokens, int(contextLen), pct, threshold, result)

	return result
}

// TrimSynthetics removes duplicate synthetic messages from history.
func (a *Assistant) TrimSynthetics() {
	a.builder.TrimDuplicateSynthetics()
}

// autoCompactAndFinalize runs auto-compaction if threshold is exceeded, then finalizes the turn.
// Uses force=true because ShouldCompact() already confirmed the threshold is exceeded —
// the compaction must succeed even with few messages (common on small-context models).
func (a *Assistant) autoCompactAndFinalize(ctx context.Context) error {
	a.builder.FinalizeAssistant() // finalize FIRST — TUI clears currentMessage, enabling standalone spinner

	if a.ShouldCompact() {
		a.send(ui.SpinnerMessage{Text: "Compacting context..."})

		if err := a.Compact(ctx, true); err != nil {
			debug.Log("[compact] auto-compact failed: %v", err)

			if errors.Is(err, ErrCompactionConfig) {
				a.send(ui.SpinnerMessage{}) // clear

				return fmt.Errorf("auto-compact aborted: %w", err)
			}

			a.send(
				ui.CommandResult{
					Message: fmt.Sprintf("auto-compact failed: %v — context remains large", err),
					Level:   ui.LevelWarn,
				},
			)
		}

		a.send(ui.SpinnerMessage{}) // clear
	}

	return nil
}

// ResolveCompaction determines which provider, model, and system prompt to use for compaction.
// Short-circuits with errNoSummarizationAgent when no agent/prompt is configured,
// then delegates to ResolveAgent and wraps errors with ErrCompactionConfig.
func (a *Assistant) ResolveCompaction(ctx context.Context) (FeatureResolution, error) {
	cfg := a.cfg.Features.Compaction

	// No configuration at all → sentinel for graceful degradation to pruning-only.
	if cfg.Prompt == "" && cfg.Agent == "" {
		return FeatureResolution{}, errNoSummarizationAgent
	}

	res, err := a.ResolveAgent(ctx, FeatureAgentConfig{
		label:      "compact",
		promptName: cfg.Prompt,
		agentName:  cfg.Agent,
		modelCache: &a.resolved.compactModel,
	})
	if err != nil {
		return FeatureResolution{}, fmt.Errorf("%w: %w", ErrCompactionConfig, err)
	}

	return res, nil
}

// Compact performs context compaction using the configured KeepLastMessages.
// When force is true (explicit /compact or emergency), KeepLastMessages is clamped
// to what's actually possible so compaction can proceed with fewer message.
func (a *Assistant) Compact(ctx context.Context, force bool) error {
	keepLast := a.cfg.Features.Compaction.KeepLastMessages

	// BeforeCompaction hook — plugins can skip compaction entirely.
	state := a.InjectorState()

	state.Compaction.Forced = force
	state.Compaction.KeepLast = keepLast

	injections := a.tools.injectors.RunBeforeCompaction(ctx, state)
	a.injectMessages(filterContentInjections(injector.Bases(injections)))

	if shouldSkipCompaction(injections) {
		debug.Log("[compact] skipped by BeforeCompaction plugin")

		return nil
	}

	return a.CompactWith(ctx, force, keepLast)
}

// CompactWithKeepLast performs forced compaction with an explicit keepLast value.
// Used by RecoverCompaction to progressively reduce preserved message.
func (a *Assistant) CompactWithKeepLast(ctx context.Context, keepLast int) error {
	return a.CompactWith(ctx, true, keepLast)
}

// RecoverCompaction performs compaction with progressive retry.
// It loops up to maxCompactionAttempts, decrementing keepLast on each failure
// or when compaction is ineffective. Returns nil on success, ErrCompactionExhausted
// when all attempts fail, or a wrapped ErrCompactionConfig on fatal config errors.
func (a *Assistant) RecoverCompaction(ctx context.Context, keepLast int) error {
	if !a.HasCompactor() {
		return errors.New("context exhausted — no compaction agent configured for recovery")
	}

	// BeforeCompaction hook — fires once before all retry attempts.
	state := a.InjectorState()

	state.Compaction.Forced = false
	state.Compaction.KeepLast = keepLast

	injections := a.tools.injectors.RunBeforeCompaction(ctx, state)
	a.injectMessages(filterContentInjections(injector.Bases(injections)))

	if shouldSkipCompaction(injections) {
		debug.Log("[compact] skipped by BeforeCompaction plugin")

		return nil
	}

	for attempt := 1; attempt <= maxCompactionAttempts; attempt++ {
		if keepLast < 0 {
			break
		}

		a.send(ui.SpinnerMessage{Text: fmt.Sprintf(
			"Compacting (attempt %d/%d, keepLast=%d)...",
			attempt, maxCompactionAttempts, keepLast)})

		preCompactLen := len(a.builder.History())

		err := a.CompactWithKeepLast(ctx, keepLast)
		if err != nil {
			debug.Log("[compact] recovery attempt %d failed (keepLast=%d): %v", attempt, keepLast, err)

			if errors.Is(err, ErrCompactionConfig) {
				a.send(ui.SpinnerMessage{}) // clear

				return fmt.Errorf("compaction aborted: %w", err)
			}

			a.send(ui.CommandResult{Message: fmt.Sprintf(
				"warning: compaction attempt %d failed (keepLast=%d): %v",
				attempt,
				keepLast,
				err,
			), Level: ui.LevelWarn})

			keepLast--

			continue
		}

		// Success — check effectiveness.
		postCompactLen := len(a.builder.History())
		if postCompactLen >= preCompactLen && keepLast > 0 {
			debug.Log("[compact] recovery attempt %d ineffective (history %d → %d), skipping to keepLast=0",
				attempt, preCompactLen, postCompactLen)

			keepLast = 0

			continue
		}

		// Effective (or already at keepLast=0): done.
		a.send(ui.SpinnerMessage{}) // clear

		return nil
	}

	a.send(ui.SpinnerMessage{}) // clear

	// Diagnostic log — enumerate remaining history so the user can see what's filling the context.
	history := a.builder.History()

	var diag strings.Builder

	fmt.Fprintf(
		&diag,
		"compaction recovery exhausted — system prompt + summary exceeds %d token context\n",
		a.agent.Model.Context,
	)

	for i, msg := range history {
		est := a.estimator.Estimate(ctx, msg.Content)
		fmt.Fprintf(&diag, "  message[%d] role=%-9s tokens=~%-6d len=%-6d", i, msg.Role, est, len(msg.Content))

		if i == 0 {
			fmt.Fprint(&diag, " (system prompt)")
		}

		fmt.Fprintln(&diag)
	}

	debug.Log("[compact] %s", diag.String())

	fmtTokens := func(n, digits int) string {
		return strings.ReplaceAll(humanize.SIWithDigits(float64(n), digits, ""), " ", "")
	}

	tokens := a.Tokens()
	ctxLen := int(a.ContextLength())

	a.send(ui.CommandResult{
		Message: fmt.Sprintf(
			"Context full (%s/%s tokens, %d messages). Compaction failed after %d attempts.\nUse /compact manually or start a new session.",
			fmtTokens(tokens, 1),
			fmtTokens(ctxLen, 0),
			len(history),
			maxCompactionAttempts,
		),
		Level: ui.LevelWarn,
	})

	return ErrCompactionExhausted
}

// CompactWith performs context compaction by summarizing old messages via an LLM call.
// When force is true, keepLast is clamped to what's actually possible.
// If no summarization agent is configured, only mechanical pruning runs.
func (a *Assistant) CompactWith(ctx context.Context, force bool, keepLast int) error {
	cfg := a.cfg.Features.Compaction

	// Resolve compaction agent/prompt first — determines whether we can summarize.
	resolved, err := a.ResolveCompaction(ctx)
	if errors.Is(err, errNoSummarizationAgent) {
		// No summarization agent — run mechanical pruning in-place, skip LLM summarization.
		if prune := cfg.Prune; prune.Mode.AtCompaction() {
			protectTokens := int(float64(a.ContextLength()) * prune.ProtectPercent / 100)
			a.builder.PruneToolResults(protectTokens, prune.ArgThreshold, a.estimator.EstimateLocal)
			debug.Log("[compact] no agent — pruned tool results in-place: protectTokens=%d argThreshold=%d",
				protectTokens, prune.ArgThreshold)
		}

		debug.Log("[compact] no agent/prompt configured — skipping summarization")

		return nil
	}

	if err != nil {
		return err
	}

	history := a.builder.History()
	preCompactLen := len(history)

	// Clamp keepLast so compaction can always proceed when there are enough messages.
	// Minimum viable: system prompt (1) + at least 1 non-system message to compact.
	maxKeep := len(history) - 2
	if maxKeep < 0 {
		return errors.New("not enough messages to compact")
	}

	if keepLast > maxKeep {
		debug.Log("[compact] clamping keepLast %d → %d (history=%d)", keepLast, maxKeep, len(history))

		keepLast = maxKeep
	}

	toCompact, preserved := splitHistory(history, keepLast)
	if len(toCompact) == 0 {
		return errors.New("nothing to compact")
	}

	debug.Log("[compact] history=%d split: compacting=%d preserved=%d keepLast=%d",
		len(history), len(toCompact), len(preserved), keepLast)

	// Log preserved message roles for debugging post-compaction issues
	if len(preserved) > 0 {
		preservedRoles := make([]string, len(preserved))
		for i, m := range preserved {
			r := string(m.Role)
			if m.Content == "" && len(m.Calls) == 0 {
				r += "(empty)"
			}

			preservedRoles[i] = r
		}

		debug.Log("[compact] preserved roles: %v", preservedRoles)
	}

	// Prune old tool results in preserved set before sending to compaction.
	if prune := cfg.Prune; prune.Mode.AtCompaction() {
		protectTokens := int(float64(a.ContextLength()) * prune.ProtectPercent / 100)

		preserved = preserved.PruneToolResults(protectTokens, prune.ArgThreshold, a.estimator.EstimateLocal)
		debug.Log(
			"[compact] pruned preserved set with protectTokens=%d argThreshold=%d",
			protectTokens,
			prune.ArgThreshold,
		)
	}

	debug.Log("[compact] sending compaction request: model=%s contextLen=%d mainContext=%d",
		resolved.mdl.Name, resolved.contextLen, a.agent.Model.Context)

	// Progressive truncation: try decreasing tool result lengths
	truncLengths := append([]int{cfg.ToolResultMaxLen}, cfg.TruncationRetries...)

	var summary string

	var lastErr error

	for _, maxLen := range truncLengths {
		var err error

		if cfg.Chunks > 1 {
			summary, err = a.CompactChunks(ctx, resolved.provider, resolved.mdl, resolved.contextLen,
				resolved.prompt, toCompact, cfg.Chunks, maxLen)
		} else {
			summary, err = a.CompactOnce(ctx, resolved.provider, resolved.mdl, resolved.contextLen,
				resolved.prompt, toCompact, maxLen, true)
		}

		if err == nil {
			break
		}

		lastErr = err
	}

	if summary == "" {
		// Fire AfterCompaction on failure path.
		compactionState := a.InjectorState()

		compactionState.Compaction.Success = false
		compactionState.Compaction.PreMessages = preCompactLen

		compactionInjections := a.tools.injectors.Run(ctx, injector.AfterCompaction, compactionState)

		var messageInjections []injector.Injection

		for _, inj := range compactionInjections {
			if inj.Content != "" || inj.DisplayOnly {
				messageInjections = append(messageInjections, inj)
			}
		}

		a.injectMessages(messageInjections)

		return fmt.Errorf("compaction: truncation attempts exhausted: %w", lastErr)
	}

	// Wrap summary in compaction markers
	wrappedSummary := wrapCompactionSummary(summary, a.tools.todo.String(), len(a.tools.todo.FindPending()))

	// Rebuild conversation: replace history, then refresh system prompt.
	// rebuildState() regenerates the system prompt with current agent/mode/tools/sandbox,
	// keeping the model oriented after compaction.
	a.session.stats.RecordCompaction()

	summaryTokens := a.estimator.Estimate(ctx, wrappedSummary)
	a.builder.RebuildAfterCompaction(wrappedSummary, summaryTokens, preserved)

	if err := a.rebuildState(); err != nil {
		return fmt.Errorf("rebuilding state after compaction: %w", err)
	}

	a.builder.DeduplicateSystemMessages()

	rebuilt := a.builder.History()
	lastRole := "none"

	if len(rebuilt) > 0 {
		lastRole = string(rebuilt[len(rebuilt)-1].Role)
	}

	debug.Log("[compact] rebuilt: %d messages, lastRole=%s, summary_len=%d",
		len(rebuilt), lastRole, len(wrappedSummary))

	// Reset per-turn token caches (will be repopulated on next chat() call).
	// Cumulative session.usage is NOT reset — those tokens were already spent.
	a.tokens.lastInput = 0
	a.tokens.lastOutput = 0
	a.tokens.lastAPIInput = 0

	a.EmitStatus()

	a.builder.AddBookmark("Context compacted")

	a.send(ui.SyntheticInjected{
		Content: wrappedSummary,
	})

	// Fire AfterCompaction on success path.
	compactionState := a.InjectorState()

	compactionState.Compaction.Success = true
	compactionState.Compaction.PreMessages = preCompactLen
	compactionState.Compaction.PostMessages = len(rebuilt)
	compactionState.Compaction.SummaryLength = len(summary)

	compactionInjections := a.tools.injectors.Run(ctx, injector.AfterCompaction, compactionState)

	var messageInjections []injector.Injection

	for _, inj := range compactionInjections {
		if inj.Content != "" || inj.DisplayOnly {
			messageInjections = append(messageInjections, inj)
		}
	}

	a.injectMessages(messageInjections)

	return nil
}

// todoState returns the current todo list wrapped in markers, or empty string if no todos exist.
func (a *Assistant) todoState() string {
	if todoStr := strings.TrimSpace(a.tools.todo.String()); todoStr != "" {
		return "\n<BEGIN TODOS>\n" + todoStr + "\n<END TODOS>"
	}

	return ""
}

// CompactOnce tries to compact messages with the given tool result truncation length.
// When includeTodos is true, the todo list is appended to the compaction prompt.
func (a *Assistant) CompactOnce(
	ctx context.Context,
	provider providers.Provider,
	mdl model.Model,
	contextLen int,
	systemPrompt string,
	toCompact message.Messages,
	maxLen int,
	includeTodos bool,
) (string, error) {
	todoState := ""

	if includeTodos {
		todoState = a.todoState()
	}

	reqMessages := buildCompactionMessages(systemPrompt, toCompact, maxLen, todoState)

	req := request.Request{
		Model:         mdl,
		Messages:      reqMessages,
		ContextLength: contextLen,
		Truncate:      true,
		Shift:         true,
		// No tools — compaction must not make tool calls
	}

	response, _, err := provider.Chat(ctx, req, noopStream)
	if err != nil {
		return "", fmt.Errorf("compaction chat: %w", err)
	}

	if len(response.Calls) > 0 {
		return "", errors.New("compaction model issued tool calls")
	}

	content := strings.TrimSpace(response.Content)
	if content == "" {
		return "", errors.New("compaction model returned empty response")
	}

	return content, nil
}

// CompactChunks splits the compactable history into n chunks and summarizes
// them sequentially, passing each chunk's summary as context to the next.
func (a *Assistant) CompactChunks(
	ctx context.Context,
	provider providers.Provider,
	mdl model.Model,
	contextLen int,
	systemPrompt string,
	toCompact message.Messages,
	n int,
	maxLen int,
) (string, error) {
	chunks := splitIntoChunks(toCompact, n)

	debug.Log("[compact] chunked compaction: %d chunks from %d messages", len(chunks), len(toCompact))

	var prevSummary string

	for i, chunk := range chunks {
		lastChunk := i == len(chunks)-1

		debug.Log("[compact] compacting chunk %d/%d: %d messages, lastChunk=%v",
			i+1, len(chunks), len(chunk), lastChunk)

		summary, err := a.attemptChunkCompaction(ctx, provider, mdl, contextLen, systemPrompt,
			chunk, maxLen, prevSummary, i+1, len(chunks), lastChunk)
		if err != nil {
			return "", fmt.Errorf("chunk %d/%d: %w", i+1, len(chunks), err)
		}

		prevSummary = summary
	}

	return prevSummary, nil
}

// attemptChunkCompaction compacts a single chunk, optionally including
// a previous summary and todo state (only for the last chunk).
func (a *Assistant) attemptChunkCompaction(
	ctx context.Context,
	provider providers.Provider,
	mdl model.Model,
	contextLen int,
	systemPrompt string,
	chunk message.Messages,
	maxLen int,
	prevSummary string,
	chunkNum, totalChunks int,
	lastChunk bool,
) (string, error) {
	// Include todo state only in the last chunk
	todoState := ""

	if lastChunk {
		todoState = a.todoState()
	}

	reqMessages := buildChunkMessages(systemPrompt, chunk, maxLen, todoState, prevSummary, chunkNum, totalChunks)

	req := request.Request{
		Model:         mdl,
		Messages:      reqMessages,
		ContextLength: contextLen,
		Truncate:      true,
		Shift:         true,
	}

	response, _, err := provider.Chat(ctx, req, noopStream)
	if err != nil {
		return "", fmt.Errorf("compaction chat (chunk %d/%d): %w", chunkNum, totalChunks, err)
	}

	if len(response.Calls) > 0 {
		return "", fmt.Errorf("compaction model issued tool calls (chunk %d/%d)", chunkNum, totalChunks)
	}

	content := strings.TrimSpace(response.Content)
	if content == "" {
		return "", fmt.Errorf("compaction model returned empty response (chunk %d/%d)", chunkNum, totalChunks)
	}

	return content, nil
}

// shouldSkipCompaction returns true if any injection requests compaction skip.
func shouldSkipCompaction(injections []injector.BeforeCompactionInjection) bool {
	for _, inj := range injections {
		if inj.Compaction != nil && inj.Compaction.Skip {
			return true
		}
	}

	return false
}

// filterContentInjections returns only injections that have content or are display-only.
func filterContentInjections(injections []injector.Injection) []injector.Injection {
	var result []injector.Injection

	for _, inj := range injections {
		if inj.Content != "" || inj.DisplayOnly {
			result = append(result, inj)
		}
	}

	return result
}
