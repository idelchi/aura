package assistant

import (
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/agent"
	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/pkg/tokens"
)

// TestCheckResultBudget uses the configured ceiling, preserves small receipts,
// and keeps fixed-token mode's explicit result limit authoritative.
func TestCheckResultBudget(t *testing.T) {
	t.Parallel()
	estimator, err := tokens.NewEstimator("rough", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	a := &Assistant{
		agent:     &agent.Agent{Model: config.Model{Context: 8192}},
		estimator: estimator,
	}
	a.cfg.Features.ToolExecution.Mode = "percentage"
	a.cfg.Features.ToolExecution.Result.MaxPercentage = 60
	a.tokens.lastInput = 3046
	_, rejected, msg := a.CheckResult(t.Context(), strings.Repeat("x", 2000))
	if !rejected || !strings.Contains(msg, "Remaining budget: 1869 tokens") {
		t.Fatalf("incorrect cap: rejected=%v message=%s", rejected, msg)
	}
	a.tokens.lastInput = 8100
	if _, rejected, _ := a.CheckResult(t.Context(), "notification sent"); rejected {
		t.Error("discarded a completed-operation receipt")
	}
	_, rejected, msg = a.CheckResult(t.Context(), strings.Repeat("x", 300))
	if !rejected || !strings.Contains(msg, "Remaining budget: 0 tokens (0%)") {
		t.Errorf("overfull budget is not clamped: %s", msg)
	}
	a.cfg.Features.ToolExecution.Mode = "tokens"
	a.cfg.Features.ToolExecution.Result.MaxTokens = 10
	if _, rejected, _ := a.CheckResult(t.Context(), "notification sent"); !rejected {
		t.Error("fixed token limit was ignored")
	}
}
