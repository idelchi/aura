package calllimit

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

// TestCounting separates failed attempts from successful raw execution outcomes.
func TestCounting(t *testing.T) {
	for _, mode := range []string{"attempt", "success", ""} {
		t.Run(mode, func(t *testing.T) {
			var limits Limiter
			rules := Rules{"Send": {Max: 1, Count: mode}}
			failure := errors.New("invalid argument")
			_, err := limits.Run(t.Context(), "Send", rules, func(context.Context) (string, error) { return "", failure })
			if !errors.Is(err, failure) {
				t.Fatal(err)
			}
			runs := 0
			execute := func(context.Context) (string, error) { runs++; return "handled", nil }
			_, err = limits.Run(t.Context(), "Send", rules, execute)
			if (err == nil) != (mode == "success") {
				t.Fatalf("mode %s: %v", mode, err)
			}
			if _, err := limits.Run(t.Context(), "Send", rules, execute); err == nil {
				t.Fatal("limit bypassed")
			}
			if !limits.Exhausted("Send", rules) || limits.Exhausted("Other", rules) {
				t.Fatal("tool availability disagrees with execution admission")
			}
			want := 0
			if mode == "success" {
				want = 1
			}
			if runs != want {
				t.Fatalf("executed %d times", runs)
			}
			// Snapshot and restore do not share mutable maps or forget consumption.
			snapshot := limits.Snapshot()
			var resumed Limiter
			resumed.Restore(snapshot)
			delete(snapshot, "Send")
			if _, err := resumed.Run(t.Context(), "Send", rules, execute); err == nil {
				t.Fatal("resume bypassed limit")
			}
			resumed.Restore(nil)
			if _, err := resumed.Run(t.Context(), "Send", rules, execute); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestConcurrentSuccess allows one successful execution, not one per queued caller.
func TestConcurrentSuccess(t *testing.T) {
	var limits Limiter
	var attempts, successes atomic.Int32
	rules := Rules{"Send": {Max: 1, Count: "success"}}
	var workers sync.WaitGroup
	for range 20 {
		workers.Go(func() {
			_, _ = limits.Run(t.Context(), "Send", rules, func(context.Context) (string, error) {
				if attempts.Add(1) <= 3 {
					return "", errors.New("retry")
				}
				successes.Add(1)
				return "sent", nil
			})
		})
	}
	workers.Wait()
	if attempts.Load() != 4 || successes.Load() != 1 {
		t.Fatalf("attempts=%d successes=%d", attempts.Load(), successes.Load())
	}
	if limits.Snapshot()["Send"] != (Usage{Attempts: 4, Successes: 1}) {
		t.Fatal(limits.Snapshot())
	}
}

// TestCancellationAndPanic preserves retry allowance when execution did not succeed.
func TestCancellationAndPanic(t *testing.T) {
	var limits Limiter
	rules := Rules{"Send": {Max: 1, Count: "success"}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := limits.Run(ctx, "Send", rules, func(context.Context) (string, error) { t.Fatal("cancelled execution"); return "", nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if len(limits.Snapshot()) != 0 {
		t.Fatal("cancelled call counted")
	}
	if _, err := limits.Run(t.Context(), "Send", rules, func(context.Context) (string, error) { panic("oops") }); err == nil {
		t.Fatal("panic lost")
	}
	if limits.Snapshot()["Send"].Successes != 0 {
		t.Fatal("panic counted as success")
	}
	if _, err := limits.Run(t.Context(), "Send", rules, func(ctx context.Context) (string, error) {
		_, err := limits.Run(ctx, "Send", rules, func(context.Context) (string, error) { t.Fatal("recursive execution"); return "", nil })
		return "", err
	}); err == nil {
		t.Fatal("recursive limit did not reject")
	}
}

// TestRulesAndUnrestrictedCalls validates policies and retains usage before opt-in.
func TestRulesAndUnrestrictedCalls(t *testing.T) {
	for _, rules := range []Rules{{"": {Max: 1}}, {"Send": {Max: 0}}, {"Send": {Max: -1}}, {"Send": {Max: 1, Count: "typo"}}, {"mcp__*": {Max: 1}}} {
		if rules.Validate() == nil {
			t.Fatalf("accepted %#v", rules)
		}
	}
	var limits Limiter
	for range 3 {
		if _, err := limits.Run(t.Context(), "Send", nil, func(context.Context) (string, error) { return "ok", nil }); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := limits.Run(t.Context(), "Send", Rules{"Send": {Max: 2}}, func(context.Context) (string, error) { t.Fatal("policy change reset usage"); return "", nil }); err == nil {
		t.Fatal("limit bypassed")
	}
}
