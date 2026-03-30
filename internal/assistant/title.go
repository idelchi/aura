package assistant

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/roles"
)

// GenerateTitle generates a short title for the current conversation.
//
// Three modes:
//   - disabled: returns the first user message, truncated to max_length
//   - no agent/prompt configured: returns the first user message (fallback)
//   - agent or prompt configured: sends first user messages to a dedicated/self-use agent
func (a *Assistant) GenerateTitle(ctx context.Context) (string, error) {
	cfg := a.cfg.Features.Title

	debug.Log("[title] GenerateTitle called: disabled=%v", cfg.IsDisabled())

	// Disabled: use first user message as title (no LLM call)
	if cfg.IsDisabled() {
		return a.titleFromFirstMessage(cfg.MaxLength), nil
	}

	// No agent or prompt configured: use first user message as fallback.
	if cfg.Agent == "" && cfg.Prompt == "" {
		return a.titleFromFirstMessage(cfg.MaxLength), nil
	}

	// Extract first user messages as context for LLM
	userContent := a.extractUserMessages(3)
	if userContent == "" {
		return "", errors.New("no user messages to generate title from")
	}

	resolved, err := a.ResolveAgent(ctx, FeatureAgentConfig{
		label:      "title",
		promptName: cfg.Prompt,
		agentName:  cfg.Agent,
		modelCache: &a.resolved.titleModel,
	})
	if err != nil {
		return "", fmt.Errorf("title generation: %w", err)
	}

	debug.Log("[title] sending title request: model=%s contextLength=%d",
		resolved.mdl.Name, resolved.contextLen)

	req := request.Request{
		Model: resolved.mdl,
		Messages: message.New(
			message.Message{Role: roles.System, Content: resolved.prompt},
			message.Message{Role: roles.User, Content: userContent},
		),
		ContextLength: resolved.contextLen,
		Truncate:      true,
		Shift:         true,
	}

	response, _, err := resolved.provider.Chat(ctx, req, noopStream)
	if err != nil {
		return "", fmt.Errorf("title generation chat: %w", err)
	}

	title := strings.TrimSpace(response.Content)
	if title == "" {
		return "", errors.New("title generation returned empty response")
	}

	if cfg.MaxLength > 0 && len(title) > cfg.MaxLength {
		title = title[:cfg.MaxLength]
	}

	return title, nil
}

// titleFromFirstMessage returns the first user message as a title, truncated to maxLength.
func (a *Assistant) titleFromFirstMessage(maxLength int) string {
	for _, msg := range a.builder.History() {
		if msg.Role == roles.User && !msg.IsInternal() && msg.Content != "" {
			title := strings.TrimSpace(msg.Content)

			// Take only the first line
			if idx := strings.IndexByte(title, '\n'); idx >= 0 {
				title = title[:idx]
			}

			if maxLength > 0 && len(title) > maxLength {
				title = title[:maxLength]
			}

			return title
		}
	}

	return ""
}

// extractUserMessages extracts the content of the first N user messages from history.
func (a *Assistant) extractUserMessages(n int) string {
	var parts []string

	count := 0

	for _, msg := range a.builder.History() {
		if msg.Role == roles.User && !msg.IsInternal() && msg.Content != "" {
			parts = append(parts, msg.Content)
			count++

			if count >= n {
				break
			}
		}
	}

	return strings.Join(parts, "\n\n")
}
