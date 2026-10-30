package assistant

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/directive"
	"github.com/idelchi/aura/internal/injector"
	"github.com/idelchi/aura/internal/tools/bash"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/aura/pkg/llm/tool/call"
	providers "github.com/idelchi/aura/pkg/providers"
)

// maxParseRetries limits how many times the loop retries after malformed tool calls.
const maxParseRetries = 3

// maxErrorRetries limits how many times the loop retries after plugin-requested error retries.
const maxErrorRetries = 3

// maxContextRecoveries bounds recovery even when estimates shrink but the provider still rejects the request.
const maxContextRecoveries = 3

// Loop processes user inputs until cancelled.
func (a *Assistant) Loop(
	ctx context.Context,
	inputs <-chan ui.UserInput,
	actions <-chan ui.UserAction,
	cancel <-chan struct{},
) error {
	defer close(a.done)
	// Done means a safe point: a turn still running (cancelled with ctx, or finishing
	// its persistence after the inputs closed) completes before the loop reports it.
	defer a.awaitTurn()

	a.ctx = ctx

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-cancel:
			if a.stream.cancel != nil {
				a.stream.cancel()
			}
		case action := <-actions:
			a.applyUIAction(ctx, action.Action)
		case input, ok := <-inputs:
			if !ok {
				return nil
			}

			a.session.stats.RecordInteraction()

			if a.handleCommand(ctx, input) {
				if a.toggles.exitRequested {
					return nil
				}

				continue
			}

			if a.stream.active {
				a.stream.pending = append(a.stream.pending, input)

				continue
			}

			a.startTurn(ctx, []ui.UserInput{input})

		case <-a.stream.done:
			a.stream.active = false
			a.stream.cancel = nil

			if len(a.stream.pending) > 0 {
				pending := a.stream.pending

				a.stream.pending = nil
				a.startTurn(ctx, pending)
			}
		}
	}
}

// awaitTurn waits for the background turn, if any, to finish.
func (a *Assistant) awaitTurn() {
	if a.stream.active {
		<-a.stream.done

		a.stream.active = false
		a.stream.cancel = nil
	}
}

// handleCommand runs input as a slash command when it is one, reporting whether it was consumed.
// Forwarded command output continues as a turn under the command's input ID.
func (a *Assistant) handleCommand(ctx context.Context, input ui.UserInput) bool {
	if input.Kind == ui.InputPrompt {
		return false
	}

	var (
		msg              string
		handled, forward bool
		err              error
	)

	if a.slashRegistry != nil {
		msg, handled, forward, err = a.slashRegistry.Handle(ctx, a, input.Text)
	}

	if !handled {
		if input.Kind != ui.InputCommand {
			return false
		}

		err = fmt.Errorf("%w: %q", ErrNotCommand, input.Text)
	}

	if err != nil {
		a.send(ui.CommandResult{Error: err})
		a.send(ui.SlashCommandHandled{Text: input.Text, InputID: input.ID, Error: err})
		a.Persist(ui.PersistCommand, inputIDs([]ui.UserInput{input})...)

		return true
	}

	if forward && msg != "" {
		a.send(ui.SlashCommandHandled{Text: input.Text, InputID: input.ID, Forwarded: true})

		forwarded := ui.UserInput{ID: input.ID, Kind: ui.InputPrompt, Text: msg}

		if a.stream.active {
			a.stream.pending = append(a.stream.pending, forwarded)

			return true
		}

		a.startTurn(ctx, []ui.UserInput{forwarded})

		return true
	}

	if msg != "" {
		a.send(ui.CommandResult{Message: msg})
	}

	a.send(ui.SlashCommandHandled{Text: input.Text, InputID: input.ID})
	a.Persist(ui.PersistCommand, inputIDs([]ui.UserInput{input})...)

	return true
}

// startTurn announces inputs and processes them as one turn in the background.
func (a *Assistant) startTurn(ctx context.Context, inputs []ui.UserInput) {
	a.send(ui.UserMessagesProcessed{Texts: inputTexts(inputs), InputIDs: inputIDs(inputs)})

	a.processWithTracking(ctx, inputs)
}

// inputTexts returns the text of each input.
func inputTexts(inputs []ui.UserInput) []string {
	texts := make([]string, len(inputs))
	for i, in := range inputs {
		texts[i] = in.Text
	}

	return texts
}

// inputIDs returns the non-empty IDs of identified inputs.
func inputIDs(inputs []ui.UserInput) []string {
	var ids []string

	for _, in := range inputs {
		if in.ID != "" {
			ids = append(ids, in.ID)
		}
	}

	return ids
}

func (a *Assistant) processWithTracking(ctx context.Context, inputs []ui.UserInput) {
	reqCtx, cancelFn := context.WithCancel(ctx)

	a.stream.cancel = cancelFn
	a.stream.active = true

	ids := inputIDs(inputs)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				err := fmt.Errorf("internal panic: %v", r)

				a.send(ui.AssistantDone{Error: err, InputIDs: ids})
				// State after a panic is not trusted for persistence.
				a.send(ui.SessionPersistFailed{
					Err:      fmt.Errorf("%w: not saved after %w", ErrPersist, err),
					Reason:   ui.PersistRun,
					InputIDs: ids,
				})
			}

			cancelFn()

			// The loop owns stream.cancel and clears it after this handoff.
			a.stream.done <- struct{}{}
		}()

		err := a.processInputs(reqCtx, inputs)

		a.send(ui.AssistantDone{Error: err, Cancelled: errors.Is(err, context.Canceled), InputIDs: ids})
		a.Persist(ui.PersistRun, ids...)
	}()
}

// chatResult classifies the outcome of handleChatError for the caller's control flow.
type chatResult int

const (
	chatContinue       chatResult = iota // retry, increment iteration normally (parse error retry)
	chatContinueNoIncr                   // retry, skip iteration increment (context exhaustion, failover, plugin retry)
	chatDone                             // error suppressed — return nil
	chatFatal                            // unrecoverable — return the error
)

func (a *Assistant) processInputs(ctx context.Context, inputs []ui.UserInput) error {
	a.send(ui.SpinnerMessage{Text: "Preparing..."})

	// Re-read all config from disk so changes made by the LLM (or the user)
	// in previous turns are picked up. Skips MCP reconnection.
	if err := a.reloadConfig(nil); err != nil {
		debug.Log("[loop] per-turn reload: %v", err)
		a.send(
			ui.CommandResult{
				Message: fmt.Sprintf("warning: config reload failed (using previous config): %v", err),
				Level:   ui.LevelWarn,
			},
		)
	}

	debug.Log("[loop] processing %d inputs", len(inputs))

	for i, input := range inputs {
		debug.Log("[loop] input[%d] id=%q: %s", i, input.ID, input.Text)
	}

	a.session.stats.RecordTurn()

	// Resolve model eagerly so capability checks (e.g., Vision) are available.
	// Non-fatal here: chat() will retry and surface the error properly.
	a.send(ui.SpinnerMessage{Text: "Resolving model..."})

	if _, err := a.ResolveModel(ctx, "chat", a.agent, &a.resolved.model); err != nil {
		debug.Log("[loop] model not resolved (will retry in chat): %v", err)
	}

	preLen := a.builder.Len()

	var anyAdded bool

	a.send(ui.SpinnerMessage{Text: "Processing input..."})

	for _, input := range inputs {
		messageID, err := a.addUserInput(ctx, input.Text)
		if err != nil {
			a.EmitStatus()

			return err
		}

		if messageID != "" {
			anyAdded = true

			a.send(ui.InputAdmitted{InputID: input.ID, MessageID: messageID})
		}
	}

	if !anyAdded {
		a.EmitStatus()

		return nil
	}

	// Capture working tree BEFORE this turn's tool calls.
	a.createSnapshot(inputs[0].Text, preLen)

	a.builder.StartAssistant()

	a.loop = loopState{}
	a.toggles.doneSignaled = false // Reset per conversation turn
	debug.Log(
		"[loop] max_steps=%d token_budget=%d",
		a.cfg.Features.ToolExecution.MaxSteps,
		a.cfg.Features.ToolExecution.TokenBudget,
	)

	// Fresh estimate after user input — replaces stale lastInputTokens from the
	// previous turn so the display reflects the new message immediately.
	a.tokens.lastInput = a.EstimateTokens(ctx)
	a.EmitStatus()

	var parseRetries int

	var errorRetries int

	var skipIterationIncrement bool

	for {
		// A cancelled turn stops at the iteration boundary: every call of the previous
		// iteration already has its result, so the history stays complete.
		if err := ctx.Err(); err != nil {
			a.builder.FinalizeAssistant()

			return err
		}

		if !skipIterationIncrement {
			a.loop.iteration++
			a.session.stats.RecordIteration()
		}

		skipIterationIncrement = false

		// Hard stop: break if we exceed MaxSteps + 1 (allows one text-only response after tools disabled)
		maxSteps := a.cfg.Features.ToolExecution.MaxSteps
		if maxSteps > 0 && a.loop.iteration > maxSteps+1 {
			debug.Log("[loop] hard stop at iteration %d (max_steps=%d)", a.loop.iteration, maxSteps)
			a.builder.FinalizeAssistant()

			return fmt.Errorf("%w (%d)", ErrMaxSteps, maxSteps)
		}

		// Hard stop: break if cumulative tokens exceed budget.
		if budget := a.cfg.Features.ToolExecution.TokenBudget; budget > 0 {
			if total := a.session.stats.Tokens.In + a.session.stats.Tokens.Out; total >= budget {
				debug.Log("[loop] token budget exceeded (%d/%d)", total, budget)
				a.builder.FinalizeAssistant()

				return fmt.Errorf("%w (%d/%d)", ErrTokenBudget, total, budget)
			}
		}

		debug.Log("[loop] iteration %d", a.loop.iteration)

		state := a.InjectorState()

		// INJECTION POINT 1: BeforeChat.
		beforeChatInjections := a.tools.injectors.RunBeforeChat(ctx, state)

		if err := injectionStop(injector.Bases(beforeChatInjections)); err != nil {
			a.builder.FinalizeAssistant()
			return err
		}

		a.injectMessages(injector.Bases(beforeChatInjections))

		// Process request modifications.
		skipChat := false

		for _, inj := range beforeChatInjections {
			if inj.Request != nil {
				if inj.Request.Skip {
					skipChat = true

					debug.Log("[loop] chat skip requested by %s", inj.Name)
				}

				if inj.Request.AppendSystem != nil {
					a.loop.appendSystem = append(a.loop.appendSystem, *inj.Request.AppendSystem)
					debug.Log("[loop] system prompt append by %s (%d chars)", inj.Name, len(*inj.Request.AppendSystem))
				}
			}
		}

		if skipChat {
			a.builder.FinalizeAssistant()

			return nil
		}

		// Trim duplicate synthetic messages when approaching context limits
		if a.ShouldTrim() {
			a.TrimSynthetics()
		}

		// Auto-compact before chat if context exceeds threshold.
		if a.ShouldCompact() {
			a.builder.FinalizeAssistant()

			if err := a.RecoverCompaction(ctx, a.cfg.Features.Compaction.KeepLastMessages); err != nil {
				return err
			}

			a.builder.StartAssistant()
		}

		a.builder.DeduplicateSystemMessages()

		a.loop.streamStarted = false // reset per chat attempt

		response, err := a.chat(ctx)
		if err == nil && ctx.Err() != nil {
			// A provider that returns normally after cancellation may hold a truncated
			// response; the turn was cancelled either way.
			err = ctx.Err()
		}

		if err != nil {
			outcome, fatalErr := a.handleChatError(ctx, err, state, &parseRetries, &errorRetries)
			switch outcome {
			case chatContinue:
				continue
			case chatContinueNoIncr:
				skipIterationIncrement = true

				continue
			case chatDone:
				return nil
			case chatFatal:
				return fatalErr
			}
		}

		parseRetries = 0
		a.loop.overflowRecoveries = 0

		// Eject synthetics after chat() saw them (one-turn-only)
		if a.loop.pendingEject {
			a.builder.EjectSyntheticMessages()

			a.loop.pendingEject = false
		}

		// Eject ephemerals unconditionally — not gated by pendingEject.
		// Ephemeral tool results (pre-execution errors) survive through chat()
		// so the model sees them, then get pruned along with their matching calls.
		a.builder.EjectEphemeralMessages()
		a.builder.PruneEmptyAssistantMessages()

		// Refresh token snapshot so AfterResponse/OnError see post-chat values.
		state.Tokens = injector.TokenSnapshot{
			Estimate: a.Tokens(),
			LastAPI:  a.tokens.lastAPIInput,
			Percent:  a.Status().Tokens.Percent,
			Max:      int(a.ContextLength()),
		}

		a.EmitStatus()

		state.Response.Content = response.Content
		state.Response.Thinking = response.Thinking
		state.Response.Calls = convertResponseCalls(response.Calls)
		state.Response.Empty = response.Content == "" && len(response.Calls) == 0
		state.Response.ContentEmpty = response.Content == ""
		if state.Response.Empty {
			a.loop.emptyResponses++
		} else {
			a.loop.emptyResponses = 0
		}
		state.Response.EmptyCount = a.loop.emptyResponses
		if len(response.Calls) == 0 {
			if err := a.agent.ResponseFormat.Validate(response.Content); err != nil {
				state.Response.ValidationError = err.Error()
			}
		}
		state.HasToolCalls = len(response.Calls) > 0

		debug.Log("[loop] response: empty=%v content_len=%d thinking_len=%d calls=%d",
			state.Response.Empty, len(response.Content), len(response.Thinking), len(response.Calls))

		// INJECTION POINT 2: AfterResponse
		afterResponseInjections := a.tools.injectors.RunAfterResponse(ctx, state)
		if err := injectionStop(injector.Bases(afterResponseInjections)); err != nil {
			a.builder.AddAssistantMessage(response)
			a.builder.FinalizeAssistant()
			return err
		}

		// Process response modifications from ALL injections.
		// Skip uses OR (any skip = skip), Content uses last-writer-wins.
		skipResponse := false

		for _, inj := range afterResponseInjections {
			if inj.Response != nil {
				if inj.Response.Skip {
					skipResponse = true

					debug.Log("[loop] response skipped by %s", inj.Name)
				}

				if inj.Response.Content != nil {
					response.Content = *inj.Response.Content
					debug.Log("[loop] response content replaced by %s", inj.Name)
				}
			}
		}

		// Silent response/filter modifications must not force another model call.
		emitted := a.injectMessages(injector.Bases(afterResponseInjections))

		if !skipResponse {
			a.builder.AddAssistantMessage(response)
		}

		if len(response.Calls) == 0 {
			// Message injections force another iteration so the model sees the nudge.
			// Silent modifications (Response-only) don't force continuation.
			if emitted {
				debug.Log("[loop] forcing continuation after AfterResponse injection")

				continue
			}

			if a.ShouldContinue() {
				debug.Log("[auto] continuing — pending=%d inProgress=%d iteration=%d/%d",
					len(a.tools.todo.FindPending()), len(a.tools.todo.FindInProgress()),
					a.loop.iteration, a.cfg.Features.ToolExecution.MaxSteps)

				continue
			}

			a.finalizeTurn()

			return nil
		}

		a.executeTools(ctx, response.Calls)

		// Prune old tool results after each iteration to keep context lean.
		if prune := a.cfg.Features.Compaction.Prune; prune.Mode.AtIteration() {
			protectTokens := int(float64(a.ContextLength()) * prune.ProtectPercent / 100)
			a.builder.PruneToolResults(protectTokens, prune.ArgThreshold, a.estimator.EstimateLocal)
			debug.Log(
				"[loop] pruned tool results with protectTokens=%d argThreshold=%d",
				protectTokens,
				prune.ArgThreshold,
			)
		}

		// Re-estimate tokens after tool results entered conversation history.
		a.tokens.lastInput = a.EstimateTokens(ctx)

		// Every call of this iteration now has its result: checkpoint the session so a
		// crash or kill loses at most the next iteration.
		if err := a.Persist(ui.PersistCheckpoint, inputIDs(inputs)...); err != nil {
			a.builder.FinalizeAssistant()

			return err
		}

		// Done tool was called — clean exit
		if a.toggles.doneSignaled {
			debug.Log("[done] LLM called Done — exiting loop")

			a.finalizeTurn()

			return nil
		}

		if len(a.stream.pending) > 0 {
			a.finalizeTurn()

			return nil
		}
	}
}

// handleChatError classifies and handles a chat() error, returning the appropriate
// loop action. Handles parse retries, context exhaustion recovery, agent failover,
// and the OnError injection pipeline (retry/skip/propagate).
//
// The order of checks matters and matches the original inline code:
//  1. Parse error — cheap retry for malformed tool call JSON
//  2. Context exhaustion — emergency compaction before giving up
//  3. Failover — try next fallback agent in chain
//  4. OnError plugins — retry/skip/propagate via injection pipeline
//
// When parse retries are exhausted, execution falls through to steps 2-4.
func (a *Assistant) handleChatError(
	ctx context.Context,
	err error,
	state *injector.State,
	parseRetries *int,
	errorRetries *int,
) (chatResult, error) {
	// 1. Parse error — malformed tool call JSON from the LLM.
	if errors.Is(err, tool.ErrToolCallParse) {
		if a.handleParseError(ctx, err, parseRetries) {
			return chatContinue, nil
		}

		// Fall through: retries exhausted → treat as generic error.
	}

	// 2. Context exhaustion — emergency compaction.
	if errors.Is(err, providers.ErrContextExhausted) {
		if a.loop.overflowRecoveries >= maxContextRecoveries {
			return chatFatal, fmt.Errorf("%w: context still rejected after %d recoveries: %w", ErrCompactionExhausted, maxContextRecoveries, err)
		}
		a.loop.overflowRecoveries++
		a.builder.FinalizeAssistant()

		if recoverErr := a.RecoverCompaction(ctx, a.cfg.Features.Compaction.KeepLastMessages); recoverErr != nil {
			return chatFatal, errors.Join(err, recoverErr)
		}

		a.builder.StartAssistant()

		return chatContinueNoIncr, nil
	}

	// 3. Failover — switch to next fallback agent.
	if a.tryFailover(err) {
		return chatContinueNoIncr, nil
	}

	// 4. OnError injection pipeline (INJECTION POINT 4).
	state.Error = err
	state.ErrorInfo.Type = classifyError(err)
	state.ErrorInfo.Retryable = isRetryable(err)

	onErrorInjections := a.tools.injectors.RunOnError(ctx, state)

	// Process error modifications. Retry/Skip use OR logic.
	retryError := false
	skipError := false

	for _, inj := range onErrorInjections {
		if inj.Error != nil {
			if inj.Error.Retry {
				retryError = true

				debug.Log("[loop] error retry requested by %s", inj.Name)
			}

			if inj.Error.Skip {
				skipError = true

				debug.Log("[loop] error skip requested by %s", inj.Name)
			}
		}
	}

	a.injectMessages(injector.Bases(onErrorInjections))

	// Retry takes precedence over skip.
	if retryError {
		*errorRetries++
		if *errorRetries > maxErrorRetries {
			debug.Log("[loop] max error retries (%d) exceeded", maxErrorRetries)
		} else {
			debug.Log("[loop] retrying chat() after plugin request (attempt %d)", *errorRetries)
			a.builder.FinalizeAssistant()
			a.builder.StartAssistant()

			return chatContinueNoIncr, nil
		}
	}

	// Skip: swallow the error — treat as if chat() returned empty.
	if skipError {
		debug.Log("[loop] error suppressed by plugin")
		a.builder.SetError(nil)
		a.builder.FinalizeAssistant()

		return chatDone, nil
	}

	// Default: show user-facing error message and propagate.
	if errMsg := providerErrorMessage(err); errMsg != "" {
		a.send(ui.CommandResult{Message: errMsg})
	} else {
		a.builder.SetError(err)
	}

	a.builder.FinalizeAssistant()

	return chatFatal, err
}

// convertResponseCalls converts call.Call (pending LLM tool requests) to injector.ToolCall.
// At AfterResponse time, calls are unexecuted — Result/Error/Duration are zero.
func convertResponseCalls(calls []call.Call) []injector.ToolCall {
	if len(calls) == 0 {
		return nil
	}

	result := make([]injector.ToolCall, len(calls))
	for i, c := range calls {
		result[i] = injector.ToolCall{
			Name: c.Name,
			Args: c.Arguments,
		}
	}

	return result
}

// ProcessInput processes a single input synchronously.
// Blocks until the assistant completes (no more tool calls) and the session is persisted.
func (a *Assistant) ProcessInput(ctx context.Context, input string) error {
	a.ctx = ctx
	a.session.stats.RecordInteraction()

	// Try slash command first
	if a.slashRegistry != nil {
		msg, handled, forward, err := a.slashRegistry.Handle(ctx, a, input)
		if handled {
			if err != nil {
				a.send(ui.CommandResult{Error: err})

				return err
			}

			if forward && msg != "" {
				a.send(ui.UserMessagesProcessed{Texts: []string{msg}})

				return a.runTurn(ctx, []ui.UserInput{{Kind: ui.InputPrompt, Text: msg}})
			}

			if msg != "" {
				a.send(ui.CommandResult{Message: msg})
			}

			return a.Persist(ui.PersistCommand)
		}
	}

	// Process as user message - blocks until assistant completes all tool calls
	a.send(ui.UserMessagesProcessed{Texts: []string{input}})

	return a.runTurn(ctx, []ui.UserInput{{Kind: ui.InputPrompt, Text: input}})
}

// runTurn processes inputs as one turn and persists the session whatever the outcome.
func (a *Assistant) runTurn(ctx context.Context, inputs []ui.UserInput) error {
	err := a.processInputs(ctx, inputs)

	if persistErr := a.Persist(ui.PersistRun, inputIDs(inputs)...); persistErr != nil {
		return errors.Join(err, persistErr)
	}

	return err
}

// ShouldContinue returns true when auto mode should force another iteration.
// This happens when Auto is enabled, there are incomplete TODOs, and the
// iteration cap has not been reached.
func (a *Assistant) ShouldContinue() bool {
	if !a.toggles.auto {
		return false
	}

	if len(a.tools.todo.FindPending()) == 0 && len(a.tools.todo.FindInProgress()) == 0 {
		return false
	}

	if maxSteps := a.cfg.Features.ToolExecution.MaxSteps; maxSteps > 0 && a.loop.iteration >= maxSteps {
		debug.Log("[auto] iteration cap reached (%d)", a.cfg.Features.ToolExecution.MaxSteps)

		return false
	}

	if budget := a.cfg.Features.ToolExecution.TokenBudget; budget > 0 {
		if a.session.stats.Tokens.In+a.session.stats.Tokens.Out >= budget {
			debug.Log("[auto] token budget reached (%d)", budget)

			return false
		}
	}

	return true
}

// addUserInput parses directives from raw input text and adds the message
// to the conversation builder, returning the admitted message's display ID.
// If images are found and the model supports vision, images are embedded directly.
// Otherwise, original text is preserved.
// Empty input is a no-op (empty ID). Rejected input returns an error wrapping
// ErrInputRejected or ErrGuardrailBlocked so automation cannot mistake it for a
// completed turn; the interactive loop reports it and stays open. Rejection happens
// before the message enters history, so it never reaches a saved session.
// Admitted input marks the session for auto-save on every input path, so a run
// that fails later still persists its history.
func (a *Assistant) addUserInput(ctx context.Context, input string) (string, error) {
	visionCfg := a.cfg.Features.Vision

	parsed := directive.Parse(ctx, input, a.effectiveWorkDir(), directive.Config{
		Image: directive.ImageConfig{
			Dimension: visionCfg.Dimension,
			Quality:   visionCfg.Quality,
		},
		RunBash: a.runDirectiveBash,
	})

	for _, w := range parsed.Warnings {
		a.send(ui.CommandResult{Message: w, Level: ui.LevelWarn})
	}

	// Prepend preamble (file contents, shell outputs) to user text
	text := parsed.Text
	if parsed.Preamble != "" {
		text = parsed.Preamble + "\n\n" + text
	}

	if strings.TrimSpace(text) == "" && !parsed.HasImages() {
		return "", nil
	}

	est, msg := a.CheckInput(ctx, text)
	if msg != "" {
		// Context too full — try compacting before rejecting
		a.send(ui.SpinnerMessage{Text: "Compacting context..."})

		compactErr := a.Compact(ctx, true)

		a.send(ui.SpinnerMessage{}) // clear

		if compactErr != nil {
			debug.Log("[input] pre-input compaction failed: %v", compactErr)
			return "", fmt.Errorf("%w: %s (compaction: %w)", ErrInputRejected, msg, compactErr)
		}

		// A successful summary or a skipped compaction does not prove admission
		// is possible. Require enough space before treating this turn as accepted.
		est, msg = a.CheckInput(ctx, text)
		if msg != "" {
			return "", fmt.Errorf("%w after compaction: %s", ErrInputRejected, msg)
		}
	}

	// Guardrail check: validate user message before it enters conversation.
	if blocked, raw, grErr := a.CheckGuardrail(ctx, "user_messages", "", text); blocked || grErr != nil {
		if grErr != nil {
			return "", fmt.Errorf("user input guardrail: %w", grErr)
		}
		return "", fmt.Errorf("%w: %s", ErrGuardrailBlocked, formatGuardrailBlock("user_messages", raw, grErr))
	}

	// Always send images when present — not all providers/models accurately report
	// vision capability, so filtering here would silently drop images for models
	// that actually support them. Let the provider handle unsupported images.
	if !parsed.HasImages() {
		id := a.builder.AddUserMessage(ctx, text, est)

		a.session.dirty = true

		return id, nil
	}

	msgImages := make(message.Images, len(parsed.Images))
	for i, img := range parsed.Images {
		msgImages[i] = message.Image{Data: []byte(img)}
	}

	id := a.builder.AddUserMessageWithImages(ctx, text, msgImages, est)

	a.session.dirty = true

	return id, nil
}

// cleanParseError extracts the meaningful part from verbose tool parse errors.
// Ollama errors look like: "error parsing tool call: raw='...', err=invalid character..."
// This returns just the suffix after ", err=" for cleaner LLM feedback.
func cleanParseError(err error) string {
	msg := err.Error()
	if _, after, ok := strings.Cut(msg, ", err="); ok {
		return after
	}

	return msg
}

// handleParseError injects error feedback for malformed tool calls.
// Returns true if the loop should continue retrying.
func (a *Assistant) handleParseError(ctx context.Context, err error, retries *int) bool {
	if *retries >= maxParseRetries {
		return false
	}

	*retries++

	a.session.stats.RecordParseRetry()
	debug.Log("[loop] tool call parse error (retry %d/%d): %v", *retries, maxParseRetries, err)

	a.builder.SetError(err)
	a.builder.FinalizeAssistant()
	a.builder.AddEphemeralToolResult(ctx, "", "", fmt.Sprintf(
		"Error: your previous response contained malformed tool calls that could not be parsed: %s. Retry using the tool-call format and argument schema advertised in this request.",
		cleanParseError(err),
	), 0)
	a.builder.StartAssistant()

	return true
}

// runDirectiveBash executes a @Bash[command] directive through the assistant's
// real bash path — with workdir, truncation, rewrite, and sandbox enforcement.
func (a *Assistant) runDirectiveBash(ctx context.Context, command string) (string, error) {
	cfg := a.cfg.Features.ToolExecution.Bash
	execCtx := tool.WithWorkDir(ctx, a.effectiveWorkDir())

	if a.toggles.sandbox {
		return a.executeSandboxed(execCtx, "Bash", map[string]any{"command": command})
	}

	return bash.New(cfg.Truncation, cfg.Rewrite).Execute(execCtx, map[string]any{"command": command})
}

// tryFailover attempts to switch to the next fallback agent after a provider failure.
// Returns true if failover succeeded and the caller should retry chat().
// Returns false if no fallbacks remain or the error is not failover-eligible.
func (a *Assistant) tryFailover(err error) bool {
	if !isFailoverEligible(err) {
		return false
	}

	if len(a.primaryFallbacks) == 0 {
		return false
	}

	if a.loop.streamStarted {
		debug.Log("[failover] skipping — streaming already started")

		return false
	}

	for a.failoverIndex < len(a.primaryFallbacks) {
		nextAgent := a.primaryFallbacks[a.failoverIndex]
		a.failoverIndex++

		prevName := a.agent.Name

		if setErr := a.setAgentFailover(nextAgent); setErr != nil {
			debug.Log("[failover] %s failed: %v", nextAgent, setErr)
			a.send(ui.CommandResult{
				Message: fmt.Sprintf("[failover] %s also unavailable: %v", nextAgent, setErr),
				Level:   ui.LevelWarn,
			})

			continue
		}

		a.send(ui.CommandResult{
			Message: fmt.Sprintf("[failover] %s unreachable, switched to %s", prevName, nextAgent),
			Level:   ui.LevelWarn,
		})

		return true
	}

	debug.Log("[failover] all %d fallbacks exhausted", len(a.primaryFallbacks))

	return false
}

// isFailoverEligible reports whether the error warrants trying a different provider.
// Content filter errors are content-specific (would likely fail on any provider).
// Canceled errors mean the user aborted (no point failing over).
func isFailoverEligible(err error) bool {
	return !errors.Is(err, providers.ErrContentFilter) &&
		!errors.Is(err, context.Canceled)
}

// providerErrorMessage returns an actionable user-facing message for typed
// provider errors. Returns "" for untyped/generic errors.
func providerErrorMessage(err error) string {
	switch {
	case errors.Is(err, providers.ErrRateLimit):
		if rle, ok := errors.AsType[*providers.RateLimitError](err); ok && rle.RetryAfter > 0 {
			return fmt.Sprintf("Rate limited. Retry after %s.", rle.RetryAfter.Round(time.Second))
		}

		return "Rate limited by provider. Wait a moment and retry."
	case errors.Is(err, providers.ErrAuth):
		return "Authentication failed. Check your API key or token."
	case errors.Is(err, providers.ErrCreditExhausted):
		return "Credits/quota exhausted. Check your billing or upgrade your plan."
	case errors.Is(err, providers.ErrModelUnavailable):
		return "Model not available or does not support the requested input type."
	case errors.Is(err, providers.ErrContentFilter):
		return "Response blocked by the provider's content filter."
	case errors.Is(err, providers.ErrNetwork):
		return "Cannot reach the provider. Check your connection and provider URL."
	case errors.Is(err, providers.ErrServerError):
		return "Provider server error. Try again later."
	default:
		return ""
	}
}
