package task

import (
	"context"
	"errors"
	"strings"
	"testing"
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
