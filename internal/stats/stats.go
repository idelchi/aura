package stats

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
)

// ToolsSnapshot holds cumulative tool invocation metrics.
type ToolsSnapshot struct {
	Calls  int            `json:"calls"`
	Errors int            `json:"errors"`
	Freq   map[string]int `json:"freq,omitempty"`
}

// TokensSnapshot holds cumulative token counts.
type TokensSnapshot struct {
	In  int `json:"in"`
	Out int `json:"out"`
}

// Stats holds cumulative session metrics.
// All mutation and read methods are safe for concurrent use.
type Stats struct {
	mu        sync.Mutex
	StartTime time.Time `json:"start_time"`

	Interactions int            `json:"interactions"`
	Turns        int            `json:"turns"`
	Iterations   int            `json:"iterations"`
	ParseRetries int            `json:"parse_retries"`
	Compactions  int            `json:"compactions"`
	Tools        ToolsSnapshot  `json:"tools"`
	Tokens       TokensSnapshot `json:"tokens"`
}

// New creates a Stats with the start time set to now.
func New() *Stats {
	return &Stats{
		StartTime: time.Now(),
		Tools:     ToolsSnapshot{Freq: make(map[string]int)},
	}
}

// RecordInteraction increments the interaction counter (every ProcessInput call).
func (s *Stats) RecordInteraction() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Interactions++
}

// RecordTurn increments the LLM turn counter.
func (s *Stats) RecordTurn() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Turns++
}

// RecordIteration increments the iteration counter.
func (s *Stats) RecordIteration() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Iterations++
}

// RecordToolCall records a successful tool execution.
func (s *Stats) RecordToolCall(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Tools.Calls++

	s.Tools.Freq[name]++
}

// RecordToolError records a failed tool execution.
func (s *Stats) RecordToolError(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Tools.Errors++

	s.Tools.Freq[name]++
}

// MergeToolCalls merges tool call counts from a subagent into the session stats.
func (s *Stats) MergeToolCalls(tools map[string]int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for name, count := range tools {
		s.Tools.Calls += count
		s.Tools.Freq[name] += count
	}
}

// RecordParseRetry increments the parse retry counter.
func (s *Stats) RecordParseRetry() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ParseRetries++
}

// RecordCompaction increments the compaction counter.
func (s *Stats) RecordCompaction() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Compactions++
}

// RecordTokens adds token counts from a single chat round.
func (s *Stats) RecordTokens(input, output int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Tokens.In += input
	s.Tokens.Out += output
}

// Duration returns the elapsed time since the session started.
func (s *Stats) Duration() time.Duration {
	return time.Since(s.StartTime).Truncate(time.Second)
}

// TopTools returns the top N most-used tools sorted by frequency.
func (s *Stats) TopTools(n int) []ToolCount {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.topToolsLocked(n)
}

// Snapshot is a frozen, mutex-free copy of Stats for safe read-only use.
type Snapshot struct {
	StartTime    time.Time      `json:"start_time"`
	Interactions int            `json:"interactions"`
	Turns        int            `json:"turns"`
	Iterations   int            `json:"iterations"`
	ParseRetries int            `json:"parse_retries"`
	Compactions  int            `json:"compactions"`
	Tools        ToolsSnapshot  `json:"tools"`
	Tokens       TokensSnapshot `json:"tokens"`
}

// Snapshot returns a frozen, mutex-free copy of the current stats.
func (s *Stats) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	freq := make(map[string]int, len(s.Tools.Freq))
	maps.Copy(freq, s.Tools.Freq)

	return Snapshot{
		StartTime:    s.StartTime,
		Interactions: s.Interactions,
		Turns:        s.Turns,
		Iterations:   s.Iterations,
		ParseRetries: s.ParseRetries,
		Compactions:  s.Compactions,
		Tools:        ToolsSnapshot{Calls: s.Tools.Calls, Errors: s.Tools.Errors, Freq: freq},
		Tokens:       TokensSnapshot{In: s.Tokens.In, Out: s.Tokens.Out},
	}
}

// Duration returns the elapsed time since the session started.
func (s Snapshot) Duration() time.Duration {
	return time.Since(s.StartTime).Truncate(time.Second)
}

// TopTools returns the top N most-used tools sorted by frequency.
func (s Snapshot) TopTools(n int) []ToolCount {
	counts := make([]ToolCount, 0, len(s.Tools.Freq))
	for name, count := range s.Tools.Freq {
		counts = append(counts, ToolCount{Name: name, Count: count})
	}

	slices.SortFunc(counts, func(a, b ToolCount) int {
		return cmp.Compare(b.Count, a.Count)
	})

	if n > 0 && len(counts) > n {
		counts = counts[:n]
	}

	return counts
}

// ToolCount pairs a tool name with its invocation count.
type ToolCount struct {
	Name  string
	Count int
}

// Display returns a formatted multi-line stats string.
// Only formats data owned by Stats — context/message counts belong to ui.Status.
func (s *Stats) Display() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var b strings.Builder

	b.WriteString("Session\n")
	b.WriteString("───────\n")
	fmt.Fprintf(&b, "  Duration:      %s\n", s.Duration())
	fmt.Fprintf(&b, "  Interactions:  %d\n", s.Interactions)
	fmt.Fprintf(&b, "  LLM turns:     %d\n", s.Turns)
	fmt.Fprintf(&b, "  Iterations:    %d\n", s.Iterations)

	used := humanize.SIWithDigits(float64(s.Tokens.In), 1, "")
	out := humanize.SIWithDigits(float64(s.Tokens.Out), 1, "")

	fmt.Fprintf(&b, "  Tokens (in):   %s\n", used)
	fmt.Fprintf(&b, "  Tokens (out):  %s\n", out)
	fmt.Fprintf(&b, "  Compactions:   %d\n", s.Compactions)

	b.WriteString("\nTools\n")
	b.WriteString("─────\n")
	fmt.Fprintf(&b, "  Total calls:   %d\n", s.Tools.Calls+s.Tools.Errors)
	fmt.Fprintf(&b, "  Succeeded:     %d\n", s.Tools.Calls)
	fmt.Fprintf(&b, "  Failed:        %d\n", s.Tools.Errors)
	fmt.Fprintf(&b, "  Parse retries: %d\n", s.ParseRetries)

	if tools := s.topToolsLocked(0); len(tools) > 0 {
		b.WriteString("\n  Frequency\n")

		for _, tc := range tools {
			fmt.Fprintf(&b, "  %-14s %d\n", tc.Name+":", tc.Count)
		}
	}

	return b.String()
}

// topToolsLocked is the lock-free inner implementation of TopTools.
// Caller must hold s.mu.
func (s *Stats) topToolsLocked(n int) []ToolCount {
	counts := make([]ToolCount, 0, len(s.Tools.Freq))
	for name, count := range s.Tools.Freq {
		counts = append(counts, ToolCount{Name: name, Count: count})
	}

	slices.SortFunc(counts, func(a, b ToolCount) int {
		return cmp.Compare(b.Count, a.Count)
	})

	if n > 0 && len(counts) > n {
		counts = counts[:n]
	}

	return counts
}
