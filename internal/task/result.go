package task

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/idelchi/aura/internal/stats"
)

// ItemResult records execution, not the correctness of a model's answer or a
// downstream side effect. An item is recorded once, after its last attempt.
type ItemResult struct {
	Duration time.Duration // elapsed item execution, including retries
	Reason   string        // machine-readable failure category
	Metrics  Metrics       // model work, excluding shell subprocesses
	Item     string        // foreach value, or task name for a non-foreach task
	Status   string        // completed, failed, cancelled or timed_out
	Attempts int           // actual attempts, including the first
	Error    string        // execution error, empty on completion
}

// Metrics records model work across commands, including session resets.
type Metrics struct {
	Iterations     int
	InputTokens    int
	OutputTokens   int
	Compactions    int
	CompactionTime time.Duration
}

// Observe adds one command's session counters without mixing /new sessions.
func (m *Metrics) Observe(before, after stats.Snapshot) {
	if !before.StartTime.Equal(after.StartTime) {
		before = stats.Snapshot{}
	}
	m.Iterations += max(0, after.Iterations-before.Iterations)
	m.InputTokens += max(0, after.Tokens.In-before.Tokens.In)
	m.OutputTokens += max(0, after.Tokens.Out-before.Tokens.Out)
	m.Compactions += max(0, after.Compactions-before.Compactions)
	m.CompactionTime += max(0, after.CompactionTime-before.CompactionTime)
}

// Result is the final execution state exposed to post hooks as .Result.
// TotalKnown is false if the foreach source could not be enumerated.
type Result struct {
	Name        string       // task name
	Status      string       // overall execution status, before post hooks
	Error       string       // overall execution error, before post hooks
	TotalKnown  bool         // whether Total covers the selected work
	Total       int          // selected items, after --start
	Completed   int          // items whose commands returned without error
	Failed      int          // attempted items that did not complete
	Unprocessed int          // selected items never attempted
	Items       []ItemResult // execution receipts, in processing order
}

// Add records one item after retries, preserving the final failure kind.
func (r *Result) Add(item string, attempts int, err error) {
	result := ItemResult{Item: item, Attempts: attempts, Status: executionStatus(err)}
	if err != nil {
		result.Error = err.Error()
		r.Failed++
	} else {
		r.Completed++
	}
	r.Items = append(r.Items, result)
}

// Finish freezes execution status before cancellation-independent cleanup starts.
func (r *Result) Finish(err error) {
	r.Status = executionStatus(err)
	if err != nil {
		r.Error = err.Error()
	}
	if r.TotalKnown {
		r.Unprocessed = max(0, r.Total-len(r.Items))
	}
}

// Summary describes execution coverage without presenting failed analysis as a
// service outage. Error bodies remain available separately for diagnostics.
func (r Result) Summary() string {
	var text strings.Builder
	fmt.Fprintf(&text, "Task %s: %s. %d completed, %d failed", r.Name, r.Status, r.Completed, r.Failed)
	if r.TotalKnown {
		fmt.Fprintf(&text, ", %d unprocessed (%d selected).", r.Unprocessed, r.Total)
	} else {
		text.WriteString("; total work unknown.")
	}
	for _, item := range r.Items {
		if item.Status != "completed" {
			fmt.Fprintf(&text, "\n%s: %s (%d attempts)", item.Item, item.Status, item.Attempts)
		}
	}
	return text.String()
}

// executionStatus gives deadlines precedence when a joined error also contains cancellation.
func executionStatus(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timed_out"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case err != nil:
		return "failed"
	default:
		return "completed"
	}
}
