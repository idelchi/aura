package assistant

import (
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/agent"
	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/pkg/llm/tool/call"
	"github.com/idelchi/aura/pkg/tokens"
)

// TestCheckResultBudget applies configured ceilings to all output, including short
// receipts; the execution outcome is represented independently of admitted output.
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
	if _, rejected, _ := a.CheckResult(t.Context(), "notification sent"); !rejected {
		t.Error("short output bypassed the configured percentage ceiling")
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

// TestRejectedOutputRetainsExecutionStatus exercises the main execution pipeline,
// not just the formatter or the budget calculation.
func TestRejectedOutputRetainsExecutionStatus(t *testing.T) {
	a := selectionAssistant(t)
	a.cfg.Features.ToolExecution.Mode = "tokens"
	a.cfg.Features.ToolExecution.Result.MaxTokens = 1
	a.executeTools(t.Context(), []call.Call{{ID: "receipt", Name: "SelectionProbe", Arguments: map[string]any{}}})
	for _, msg := range a.builder.History() {
		if msg.ToolCallID == "receipt" {
			if !strings.Contains(msg.Content, "executed successfully") || strings.HasPrefix(msg.Content, "Error:") {
				t.Fatal(msg.Content)
			}
			return
		}
	}
	t.Fatal("missing execution receipt")
}
