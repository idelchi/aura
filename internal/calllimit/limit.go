// Package calllimit enforces conversation-scoped tool execution budgets.
package calllimit

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"sync"

	"golang.org/x/sync/semaphore"
)

// Rule limits either execution attempts or successful executions of one tool.
type Rule struct {
	// Max is the positive number of calls permitted in a conversation.
	Max int `yaml:"max"`
	// Count is "attempt" (also the default) or "success".
	Count string `yaml:"count"`
}

// Counting returns the effective counting policy.
func (r Rule) Counting() string {
	if r.Count == "" {
		return "attempt"
	}
	return r.Count
}

// Rules maps exact registered tool names to budgets; absent tools are unlimited.
type Rules map[string]Rule

// Validate rejects ambiguous names, non-positive limits and unknown policies.
func (rs Rules) Validate() error {
	for name, r := range rs {
		if strings.TrimSpace(name) != name || name == "" || strings.ContainsAny(name, "*?[") {
			return fmt.Errorf("tools.call_limits: %q must be an exact tool name", name)
		}
		if r.Max < 1 {
			return fmt.Errorf("tools.call_limits.%s.max must be positive", name)
		}
		if r.Counting() != "attempt" && r.Counting() != "success" {
			return fmt.Errorf("tools.call_limits.%s.count must be attempt or success", name)
		}
	}
	return nil
}

// Usage records raw executions independently of the current counting policy.
type Usage struct {
	Attempts  int `json:"attempts"`
	Successes int `json:"successes"`
}

// Limiter owns counters and per-tool serialization for one conversation.
// Its zero value is ready for use. Do not copy a used Limiter.
type Limiter struct {
	mu    sync.Mutex
	usage map[string]Usage
	gates map[string]*semaphore.Weighted
}

// heldGate tracks enclosing limited invocations to reject recursive deadlocks.
type heldGate struct {
	gate   *semaphore.Weighted
	parent *heldGate
}

// gateKey scopes execution ancestry stored in a context.
type gateKey struct{}

// Run admits and records execution. Preflight rejections belong outside this method.
// Limited calls serialize by tool, so a failed call can release its success
// allowance before the next queued call is considered. Other tools stay concurrent.
func (l *Limiter) Run(ctx context.Context, name string, rules Rules, execute func(context.Context) (string, error)) (output string, err error) {
	rule, limited := rules[name]
	if limited {
		l.mu.Lock()
		if l.gates == nil {
			l.gates = make(map[string]*semaphore.Weighted)
		}
		gate := l.gates[name]
		if gate == nil {
			gate = semaphore.NewWeighted(1)
			l.gates[name] = gate
		}
		l.mu.Unlock()
		parent, _ := ctx.Value(gateKey{}).(*heldGate)
		for held := parent; held != nil; held = held.parent {
			if held.gate == gate {
				return "", fmt.Errorf("tool %q not executed: recursive invocation of a call-limited tool", name)
			}
		}
		if err := gate.Acquire(ctx, 1); err != nil {
			return "", err
		}
		defer gate.Release(1)
		ctx = context.WithValue(ctx, gateKey{}, &heldGate{gate: gate, parent: parent})
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	l.mu.Lock()
	usage := l.usage[name]
	used := usage.Attempts
	if rule.Counting() == "success" {
		used = usage.Successes
	}
	if limited && used >= rule.Max {
		l.mu.Unlock()
		return "", fmt.Errorf("tool %q not executed: conversation call limit reached (%d %s calls). Do not repeat this call", name, rule.Max, rule.Counting())
	}
	if l.usage == nil {
		l.usage = make(map[string]Usage)
	}
	usage.Attempts++
	l.usage[name] = usage
	l.mu.Unlock()

	// Count the raw outcome before result pruning or post-processing.
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("tool panicked: %v", recovered)
		}
		if err == nil {
			l.mu.Lock()
			usage := l.usage[name]
			usage.Successes++
			l.usage[name] = usage
			l.mu.Unlock()
		}
	}()
	return execute(ctx)
}

// Snapshot returns independent counters suitable for saved-session metadata.
func (l *Limiter) Snapshot() map[string]Usage {
	l.mu.Lock()
	defer l.mu.Unlock()
	return maps.Clone(l.usage)
}

// Restore replaces counters between executions; nil starts a fresh conversation.
func (l *Limiter) Restore(usage map[string]Usage) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.usage = maps.Clone(usage)
}
