package tasks

import (
	"context"
	"errors"
	"time"

	"github.com/idelchi/aura/internal/assistant"
	"github.com/idelchi/aura/internal/task"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/aura/pkg/providers"
)

// recordItem attaches execution measurements and a stable reason to the receipt.
func recordItem(result *task.Result, item string, attempts int, err error, started time.Time, metrics task.Metrics) {
	result.Add(item, attempts, err)
	receipt := &result.Items[len(result.Items)-1]
	receipt.Duration = time.Since(started)
	receipt.Metrics = metrics
	receipt.Reason = failureReason(err)
}

// failureReason classifies typed errors without scraping diagnostic messages.
func failureReason(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, assistant.ErrMaxSteps):
		return "max_steps"
	case errors.Is(err, assistant.ErrTokenBudget):
		return "token_budget"
	case errors.Is(err, assistant.ErrPolicyStopped):
		return "policy_stopped"
	case errors.Is(err, assistant.ErrCompactionExhausted):
		if errors.Is(err, context.DeadlineExceeded) {
			return "compaction_deadline"
		}
		return "compaction_exhausted"
	case errors.Is(err, tool.ErrToolCallParse):
		return "tool_parse"
	case errors.Is(err, providers.ErrContextExhausted):
		return "context_overflow"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	default:
		return "execution_error"
	}
}
