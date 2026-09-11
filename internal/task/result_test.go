package task

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/idelchi/aura/internal/stats"
)

// TestResultCoverage keeps execution, retries, skipped work and finalization
// failures separate from claims about model answers or notification delivery.
func TestResultCoverage(t *testing.T) {
	result := Result{Name: "logs", TotalKnown: true, Total: 3}
	result.Add("healthy", 1, nil)
	result.Add("unreadable", 2, errors.New("compaction failed"))
	result.Finish(context.DeadlineExceeded)
	if result.Completed != 1 || result.Failed != 1 || result.Unprocessed != 1 || result.Status != "timed_out" || result.Items[1].Attempts != 2 {
		t.Fatalf("incorrect coverage: %+v", result)
	}
	if !strings.Contains(result.Summary(), "unreadable: failed (2 attempts)") {
		t.Fatal(result.Summary())
	}
	// A failed setup has no known item count; zero does not imply an empty scan.
	setup := Result{Name: "logs"}
	setup.Finish(errors.New("source unavailable"))
	if setup.Status != "failed" || !strings.Contains(setup.Summary(), "total work unknown") {
		t.Fatal(setup.Summary())
	}
	// All items can succeed and the final synthesis can still fail.
	final := Result{Name: "logs", TotalKnown: true, Total: 1}
	final.Add("one", 1, nil)
	final.Finish(context.Canceled)
	if final.Completed != 1 || final.Failed != 0 || final.Status != "cancelled" {
		t.Fatalf("finalization failure lost: %+v", final)
	}
}

// TestMetricsObserve counts work once across ordinary commands and session resets.
func TestMetricsObserve(t *testing.T) {
	started := time.Now()
	before := stats.Snapshot{StartTime: started, Iterations: 2, Tokens: stats.TokensSnapshot{In: 100, Out: 10}}
	after := stats.Snapshot{StartTime: started, Iterations: 3, Tokens: stats.TokensSnapshot{In: 150, Out: 20}, CompactionTime: time.Second}
	var metrics Metrics
	metrics.Observe(before, after)
	metrics.Observe(after, after)
	if metrics.Iterations != 1 || metrics.InputTokens != 50 || metrics.OutputTokens != 10 || metrics.CompactionTime != time.Second {
		t.Fatalf("incorrect command delta: %+v", metrics)
	}
	reset := stats.Snapshot{StartTime: started.Add(time.Second), Iterations: 1, Tokens: stats.TokensSnapshot{In: 25, Out: 5}}
	metrics.Observe(after, reset)
	if metrics.Iterations != 2 || metrics.InputTokens != 75 || metrics.OutputTokens != 15 {
		t.Fatalf("lost work after new session: %+v", metrics)
	}
}
