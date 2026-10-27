package assistant

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	humanize "github.com/dustin/go-humanize"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/injector"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/stream"
	"github.com/idelchi/aura/pkg/llm/usage"
	providererrors "github.com/idelchi/aura/pkg/providers"
)

// ErrCompactionConfig indicates a configuration error that will never succeed on retry.
var ErrCompactionConfig = errors.New("compaction misconfigured")

// ErrCompactionInputBudget indicates that even the smallest configured tool-result
// setting leaves a summarization request above its estimated input budget. Retrying
// with fewer preserved messages would only enlarge the request, so recovery stops.
var ErrCompactionInputBudget = errors.New("compaction request exceeds its input budget")

// ErrCompactionSummaryCutOff indicates that the summarizer reported stopping at a length
// limit, so its summary may be incomplete and must not replace the history.
var ErrCompactionSummaryCutOff = errors.New("compaction summary was cut off at a length limit")

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

// finalizeTurn finalizes a turn without summarizing disposable history.
// Admission and automatic compaction run before the next model request, if any.
func (a *Assistant) finalizeTurn() {
	a.builder.FinalizeAssistant()
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
	a.injectMessages(injector.Bases(injections))

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
	if err := ctx.Err(); err != nil {
		return err
	}
	if !a.HasCompactor() {
		return errors.New("context exhausted — no compaction agent configured for recovery")
	}
	if timeout := a.cfg.Features.Compaction.Timeout; timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// BeforeCompaction hook — fires once before all retry attempts.
	state := a.InjectorState()

	state.Compaction.Forced = false
	state.Compaction.KeepLast = keepLast

	injections := a.tools.injectors.RunBeforeCompaction(ctx, state)
	a.injectMessages(injector.Bases(injections))

	if shouldSkipCompaction(injections) {
		debug.Log("[compact] skipped by BeforeCompaction plugin")

		return fmt.Errorf("%w: recovery skipped by BeforeCompaction plugin", ErrCompactionExhausted)
	}

	attempts := 0
	for attempt := 1; attempt <= maxCompactionAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if keepLast < 0 {
			break
		}
		attempts = attempt

		a.send(ui.SpinnerMessage{Text: fmt.Sprintf(
			"Compacting (attempt %d/%d, keepLast=%d)...",
			attempt, maxCompactionAttempts, keepLast)})

		preCompactTokens := a.EstimateTokens(ctx)

		err := a.CompactWithKeepLast(ctx, keepLast)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			debug.Log("[compact] recovery attempt %d failed (keepLast=%d): %v", attempt, keepLast, err)

			if errors.Is(err, ErrCompactionConfig) {
				a.send(ui.SpinnerMessage{}) // clear

				return fmt.Errorf("compaction aborted: %w", err)
			}
			// A smaller transcript can fix context overflow, not an invalid
			// summary, authorization error, or exhausted transport retry.
			if !errors.Is(err, providererrors.ErrContextExhausted) {
				return fmt.Errorf("%w: %w", ErrCompactionExhausted, err)
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

		// Fewer messages do not imply a smaller request (one summary may be larger).
		postCompactTokens := a.EstimateTokens(ctx)
		if postCompactTokens >= preCompactTokens {
			debug.Log("[compact] recovery attempt %d ineffective (tokens %d → %d)",
				attempt, preCompactTokens, postCompactTokens)
			if keepLast == 0 {
				break
			}

			keepLast = 0

			continue
		}

		// The request became smaller; the next provider call remains the final check.
		a.send(ui.SpinnerMessage{}) // clear

		return nil
	}

	a.send(ui.SpinnerMessage{}) // clear

	// Diagnostic log — enumerate remaining history so the user can see what's filling the context.
	history := a.builder.History()

	var diag strings.Builder

	fmt.Fprintf(
		&diag,
		"compaction recovery exhausted — no effective reduction for %d token context\n",
		a.agent.Model.Context,
	)

	for i, msg := range history {
		est := a.estimator.EstimateLocal(msg.Content)
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
			attempts,
		),
		Level: ui.LevelWarn,
	})

	return ErrCompactionExhausted
}

// CompactWith performs context compaction by summarizing old messages via an LLM call.
// When force is true, keepLast is clamped to what's actually possible.
// If no summarization agent is configured, only mechanical pruning runs.
func (a *Assistant) CompactWith(ctx context.Context, force bool, keepLast int) error {
	started := time.Now()
	defer func() { a.session.stats.RecordCompactionTime(time.Since(started)) }()
	cfg := a.cfg.Features.Compaction
	if cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}

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
	// Count the same non-internal messages as splitHistory: the system prompt
	// plus at least one message to compact. Bookmarks must not consume keepLast.
	maxKeep := len(history.WithoutInternalMessages()) - 2
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

	budgetTokens, basis, err := cfg.InputBudget(resolved.ContextLength(), resolved.generation.OutputLimit())
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCompactionConfig, err)
	}

	budget := compactionBudget{Tokens: budgetTokens, Basis: basis}

	debug.Log("[compact] sending compaction request: model=%s contextLen=%d mainContext=%d budget=%s",
		resolved.mdl.Name, resolved.ContextLength(), a.agent.Model.Context, budget)

	// Each request uses the largest tool-result setting that fits; see summarize.
	lengths := append([]int{cfg.ToolResultMaxLen}, cfg.TruncationRetries...)

	var summary string

	if cfg.Chunks > 1 {
		summary, err = a.CompactChunks(ctx, resolved, toCompact, cfg.Chunks, lengths, budget)
	} else {
		summary, err = a.CompactOnce(ctx, resolved, toCompact, lengths, budget, true)
	}

	if err != nil {
		// Fire AfterCompaction on failure path. History is unchanged.
		compactionState := a.InjectorState()

		compactionState.Compaction.Success = false
		compactionState.Compaction.PreMessages = preCompactLen

		compactionInjections := a.tools.injectors.Run(ctx, injector.AfterCompaction, compactionState)

		a.injectMessages(compactionInjections)

		return fmt.Errorf("compaction: %w", err)
	}

	// Wrap summary in compaction markers
	wrappedSummary := wrapCompactionSummary(summary, a.tools.todo.String(), len(a.tools.todo.FindPending()))
	wrappedSummary += compactionReceipts(a.loop.toolHistory)

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

	a.injectMessages(compactionInjections)

	return nil
}

// todoState returns the current todo list wrapped in markers, or empty string if no todos exist.
func (a *Assistant) todoState() string {
	if todoStr := strings.TrimSpace(a.tools.todo.String()); todoStr != "" {
		return "\n<BEGIN TODOS>\n" + todoStr + "\n<END TODOS>"
	}

	return ""
}

// CompactOnce summarizes messages in one request, using the largest tool-result
// length in lengths that fits budget (see summarize).
// When includeTodos is true, the todo list is appended to the compaction prompt.
func (a *Assistant) CompactOnce(
	ctx context.Context,
	resolved FeatureResolution,
	toCompact message.Messages,
	lengths []int,
	budget compactionBudget,
	includeTodos bool,
) (string, error) {
	todoState := ""

	if includeTodos {
		todoState = a.todoState()
	}

	return a.summarize(ctx, resolved, toCompact, lengths, budget, func(maxLen int) message.Messages {
		return buildCompactionMessages(resolved.prompt, toCompact, maxLen, todoState)
	})
}

// CompactChunks splits the compactable history into n chunks and summarizes
// them sequentially, passing each chunk's summary as context to the next.
// Each chunk request independently uses the largest fitting tool-result length.
func (a *Assistant) CompactChunks(
	ctx context.Context,
	resolved FeatureResolution,
	toCompact message.Messages,
	n int,
	lengths []int,
	budget compactionBudget,
) (string, error) {
	chunks := splitIntoChunks(toCompact, n)

	debug.Log("[compact] chunked compaction: %d chunks from %d messages", len(chunks), len(toCompact))

	var prevSummary string

	for i, chunk := range chunks {
		lastChunk := i == len(chunks)-1

		debug.Log("[compact] compacting chunk %d/%d: %d messages, lastChunk=%v",
			i+1, len(chunks), len(chunk), lastChunk)

		// Include todo state only in the last chunk.
		todoState := ""
		if lastChunk {
			todoState = a.todoState()
		}

		chunkNum, total := i+1, len(chunks)

		summary, err := a.summarize(ctx, resolved, chunk, lengths, budget, func(maxLen int) message.Messages {
			return buildChunkMessages(resolved.prompt, chunk, maxLen, todoState, prevSummary, chunkNum, total)
		})
		if err != nil {
			return "", fmt.Errorf("chunk %d/%d: %w", chunkNum, total, err)
		}

		prevSummary = summary
	}

	return prevSummary, nil
}

// summarize sends one summarization request. It walks lengths in order, building
// the request with each tool-result length: a request whose estimated input exceeds
// budget is skipped without a call. A provider context overflow, or a summary the
// provider reports as cut off at a length limit, moves on to the next length, because
// less input leaves more room; a cut-off that used up a configured max_output_tokens
// fails at once, because less input would not help. Any other failure is returned at
// once. A summary is accepted only when the provider does not report it cut off; an
// unreported finish reason is accepted but not verified. User and assistant text is
// never shortened, so when even the last length does not fit, summarize fails with
// ErrCompactionInputBudget. After the last length, the last overflow or cut-off is
// returned. Failures never change the conversation.
func (a *Assistant) summarize(
	ctx context.Context,
	resolved FeatureResolution,
	source message.Messages,
	lengths []int,
	budget compactionBudget,
	build func(maxLen int) message.Messages,
) (string, error) {
	var retry error

	smallest := 0

	for _, maxLen := range lengths {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		reqMessages := build(maxLen)
		estimate := a.estimateRequest(ctx, reqMessages)
		cut := source.WithoutEphemeralMessages().ResultsBeyond(maxLen)

		if !budget.Admits(estimate) {
			debug.Log("[compact] request skipped: tool_result_max_length=%d estimate=%d budget=%s excerpted_or_omitted=%d",
				maxLen, estimate, budget, cut)

			smallest = estimate

			continue
		}

		debug.Log("[compact] request: tool_result_max_length=%d estimate=%d budget=%s excerpted_or_omitted=%d",
			maxLen, estimate, budget, cut)

		response, used, err := a.requestSummary(ctx, resolved, reqMessages)

		switch {
		case errors.Is(err, providererrors.ErrContextExhausted):
			debug.Log("[compact] provider overflow at tool_result_max_length=%d: %v", maxLen, err)

			retry = err

			continue
		case err != nil:
			return "", err
		case len(response.Calls) > 0:
			return "", errors.New("compaction model issued tool calls")
		case response.Finish == message.FinishError || response.Finish == message.FinishContentFilter ||
			response.Finish == message.FinishToolCalls:
			return "", fmt.Errorf("compaction model stopped without a complete summary: finish %s", response.Finish)
		case response.Finish.IsCutOff():
			cutOff := fmt.Errorf("%w: finish %s at tool_result_max_length %d after %d output tokens",
				ErrCompactionSummaryCutOff, response.Finish, maxLen, used.Output)

			if limit := resolved.generation.OutputLimit(); limit != nil && used.Output >= *limit {
				return "", fmt.Errorf("%w; max_output_tokens %d was used up", cutOff, *limit)
			}

			debug.Log("[compact] %v", cutOff)

			retry = cutOff

			continue
		}

		content := strings.TrimSpace(response.Content)
		if content == "" {
			return "", errors.New("compaction model returned empty response")
		}

		debug.Log("[compact] summary accepted: finish=%s output_tokens=%d", response.Finish, used.Output)

		return content, nil
	}

	if retry != nil {
		return "", retry
	}

	return "", fmt.Errorf(
		"%w: an estimated %d input tokens with tool_result_max_length %d exceeds %d (%s); "+
			"increase compaction.chunks or start a new conversation",
		ErrCompactionInputBudget, smallest, lengths[len(lengths)-1], budget.Tokens, budget.Basis)
}

// estimateRequest estimates a summarization request's input. Messages are rendered
// as text and counted with the session estimator plus the observed provider overhead;
// it is an approximation, not the provider's tokenizer.
func (a *Assistant) estimateRequest(ctx context.Context, reqMessages message.Messages) int {
	return a.estimator.Estimate(ctx, reqMessages.Render()) + a.tokens.overhead
}

// requestSummary performs one summarization call without tools; summarize validates
// the response and its reported finish reason.
func (a *Assistant) requestSummary(
	ctx context.Context,
	resolved FeatureResolution,
	reqMessages message.Messages,
) (message.Message, usage.Usage, error) {
	req := request.Request{
		Model:         resolved.mdl,
		Think:         resolved.think,
		Generation:    resolved.generation,
		Messages:      reqMessages,
		ContextLength: resolved.contextLen,
		// Reject overflow instead of letting the provider silently discard source.
		Truncate: false,
		Shift:    false,
		// No tools — compaction must not make tool calls
	}

	response, used, err := resolved.provider.Chat(ctx, req, noopStream)
	if err != nil {
		return message.Message{}, usage.Usage{}, fmt.Errorf("compaction chat: %w", err)
	}

	return response, used, nil
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
