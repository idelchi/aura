package tasks

import (
	"context"
	"errors"
	"testing"

	"github.com/idelchi/aura/internal/assistant"
)

func TestFailureReasonPreservesPhase(t *testing.T) {
	if got := failureReason(errors.Join(assistant.ErrCompactionExhausted, context.DeadlineExceeded)); got != "compaction_deadline" {
		t.Fatal(got)
	}
	if got := failureReason(context.DeadlineExceeded); got != "deadline" {
		t.Fatal(got)
	}
	if got := failureReason(assistant.ErrPolicyStopped); got != "policy_stopped" {
		t.Fatal(got)
	}
}
