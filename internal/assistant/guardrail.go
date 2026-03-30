package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/responseformat"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/wildcard"
)

// scopeState holds the cached model for a single guardrail scope.
// Agents are created on-demand via ResolveAgent.
type scopeState struct {
	model *model.Model
}

// guardrailState groups all guardrail-related runtime state.
type guardrailState struct {
	toolCalls    scopeState
	userMessages scopeState
}

// ResolveGuardrail determines which provider, model, and system prompt to use for a guardrail
// check in the given scope. Dispatches scope-specific config then delegates to ResolveAgent.
func (a *Assistant) ResolveGuardrail(ctx context.Context, scope string) (FeatureResolution, error) {
	cfg := a.cfg.Features.Guardrail

	var (
		entry      config.GuardrailScopeEntry
		modelCache **model.Model
	)

	switch scope {
	case "tool_calls":
		entry = cfg.Scope.ToolCalls
		modelCache = &a.resolved.guardrail.toolCalls.model
	case "user_messages":
		entry = cfg.Scope.UserMessages
		modelCache = &a.resolved.guardrail.userMessages.model
	default:
		return FeatureResolution{}, fmt.Errorf("unknown guardrail scope %q", scope)
	}

	return a.ResolveAgent(ctx, FeatureAgentConfig{
		label:      "guardrail-" + scope,
		promptName: entry.Prompt,
		agentName:  entry.Agent,
		modelCache: modelCache,
	})
}

// CheckGuardrail runs a guardrail check for the given scope/content.
// Returns (blocked, raw response, error).
// Short-circuits if mode=="" or scope not active.
// For tool_calls: also checks ShouldCheck before calling the model.
func (a *Assistant) CheckGuardrail(ctx context.Context, scope, toolName, content string) (bool, string, error) {
	cfg := a.cfg.Features.Guardrail

	// Short-circuit if guardrail is disabled.
	if cfg.Mode == "" {
		return false, "", nil
	}

	// Check scope is active.
	var entry config.GuardrailScopeEntry

	switch scope {
	case "tool_calls":
		entry = cfg.Scope.ToolCalls
	case "user_messages":
		entry = cfg.Scope.UserMessages
	default:
		return false, "", fmt.Errorf("unknown guardrail scope %q", scope)
	}

	if !entry.Active() {
		return false, "", nil
	}

	// For tool_calls: also filter by tool name.
	if scope == "tool_calls" {
		if !a.ShouldCheck(toolName) {
			debug.Log("[guardrail] skipping %s (tool filter excluded)", toolName)

			return false, "", nil
		}
	}

	// Show spinner while guardrail is running.
	a.send(ui.SpinnerMessage{Text: "Checking guardrail…"})
	defer a.send(ui.SpinnerMessage{}) // clear

	// Resolve provider/model/prompt.
	resolved, err := a.ResolveGuardrail(ctx, scope)
	if err != nil {
		if cfg.OnError == "allow" {
			debug.Log("[guardrail] resolve error (on_error=allow, proceeding): %v", err)

			return false, "", nil
		}

		return true, "", fmt.Errorf("guardrail resolve: %w", err)
	}

	// Build request.
	req := request.Request{
		Model: resolved.mdl,
		Messages: message.New(
			message.Message{Role: roles.System, Content: resolved.prompt},
			message.Message{Role: roles.User, Content: content},
		),
		ContextLength: resolved.contextLen,
		Truncate:      true,
		Shift:         true,
		// No tools — guardrail must not make tool calls
		ResponseFormat: &responseformat.ResponseFormat{
			Type: responseformat.JSONSchema,
			Name: "guardrail_result",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"result": map[string]any{
						"type": "string",
						"enum": []any{guardrailSafe, guardrailUnsafe},
					},
					"reason": map[string]any{
						"type": "string",
					},
				},
				"required":             []any{"result"},
				"additionalProperties": false,
			},
			Strict: true,
		},
	}

	// Apply timeout.
	timeoutCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	response, _, err := resolved.provider.Chat(timeoutCtx, req, noopStream)
	if err != nil {
		debug.Log("[guardrail] chat error: %v", err)

		if cfg.OnError == "allow" {
			a.builder.AddDisplayMessage(ctx, roles.System,
				fmt.Sprintf("[guardrail] error — proceeding (fail-open): scope=%s error=%v", scope, err))

			return false, "", nil
		}

		a.builder.AddDisplayMessage(ctx, roles.System,
			fmt.Sprintf("[guardrail] error — blocking (fail-closed): scope=%s error=%v", scope, err))

		return true, "", nil
	}

	raw := strings.TrimSpace(response.Content)
	result := parseGuardrailResponse(raw)
	safe := result.Result == guardrailSafe

	debug.Log("[guardrail] scope=%s tool=%s safe=%v reason=%q raw=%q", scope, toolName, safe, result.Reason, raw)

	if safe {
		return false, raw, nil
	}

	// Flagged as unsafe.
	if cfg.Mode == "log" {
		a.builder.AddDisplayMessage(ctx, roles.System,
			fmt.Sprintf("[guardrail] flagged: scope=%s tool=%s response=%s", scope, toolName, raw))

		return false, raw, nil
	}

	// mode == "block" — return reason for the block message.
	return true, result.Reason, nil
}

const (
	guardrailSafe   = "safe"
	guardrailUnsafe = "unsafe"
)

// guardrailResult is the JSON response from a structured-output guardrail check.
type guardrailResult struct {
	Result string `json:"result"`
	Reason string `json:"reason,omitempty"`
}

// parseGuardrailResponse parses the guardrail model's response into a guardrailResult.
// Tries JSON first (structured output path), then falls back to first-token parsing
// for providers/models that ignore ResponseFormat. Fail-closed on ambiguity.
func parseGuardrailResponse(content string) guardrailResult {
	// Try JSON parse first (structured output path).
	var result guardrailResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &result); err == nil {
		result.Result = strings.ToLower(result.Result)
		return result
	}

	// Fallback: first-token parse (for providers/models that ignore ResponseFormat).
	// Reason is unavailable in this path.
	fields := strings.Fields(content)
	if len(fields) == 0 {
		return guardrailResult{Result: guardrailUnsafe}
	}

	return guardrailResult{Result: strings.ToLower(fields[0])}
}

// ShouldCheck returns whether a tool should be checked by the guardrail.
// Uses wildcard.MatchAny with the configured enabled/disabled lists.
func (a *Assistant) ShouldCheck(toolName string) bool {
	cfg := a.cfg.Features.Guardrail.Tools

	// If disabled list matches, skip.
	if len(cfg.Disabled) > 0 && wildcard.MatchAny(toolName, cfg.Disabled...) {
		return false
	}

	// If enabled list is set, only check matching tools.
	if len(cfg.Enabled) > 0 {
		return wildcard.MatchAny(toolName, cfg.Enabled...)
	}

	// No filters — check all tools.
	return true
}

// formatToolCall formats a tool call for guardrail evaluation.
func formatToolCall(name string, args map[string]any) string {
	argsJSON, err := json.Marshal(args)
	if err != nil {
		argsJSON = fmt.Appendf(nil, "%v", args)
	}

	return fmt.Sprintf("Tool: %s\nArguments: %s", name, string(argsJSON))
}

// formatGuardrailBlock formats a descriptive message when a guardrail check blocks an action.
// Distinguishes three blocked subcases: config/resolve error, provider-unreachable fail-closed, and model-flagged.
// The reason parameter is the guardrail model's explanation (may be empty for providers that don't support it).
func formatGuardrailBlock(name, reason string, err error) string {
	if err != nil {
		return fmt.Sprintf("Guardrail check failed for %q: %v", name, err)
	}

	if reason == "" {
		return fmt.Sprintf("Guardrail blocked %q — flagged as potentially unsafe", name)
	}

	return fmt.Sprintf("Guardrail blocked %q — %s", name, reason)
}
