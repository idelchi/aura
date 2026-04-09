package stats_test

import (
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/stats"
)

func TestNew(t *testing.T) {
	t.Parallel()

	s := stats.New()

	if s == nil {
		t.Fatal("New() returned nil")
	}

	if s.StartTime.IsZero() {
		t.Errorf("StartTime is zero, want non-zero")
	}

	if s.Interactions != 0 {
		t.Errorf("Interactions = %d, want 0", s.Interactions)
	}

	if s.Turns != 0 {
		t.Errorf("Turns = %d, want 0", s.Turns)
	}

	if s.Iterations != 0 {
		t.Errorf("Iterations = %d, want 0", s.Iterations)
	}

	if s.Tools.Calls != 0 {
		t.Errorf("Tools.Calls = %d, want 0", s.Tools.Calls)
	}

	if s.Tools.Errors != 0 {
		t.Errorf("Tools.Errors = %d, want 0", s.Tools.Errors)
	}

	if s.ParseRetries != 0 {
		t.Errorf("ParseRetries = %d, want 0", s.ParseRetries)
	}

	if s.Compactions != 0 {
		t.Errorf("Compactions = %d, want 0", s.Compactions)
	}

	if s.Tokens.In != 0 {
		t.Errorf("Tokens.In = %d, want 0", s.Tokens.In)
	}

	if s.Tokens.Out != 0 {
		t.Errorf("Tokens.Out = %d, want 0", s.Tokens.Out)
	}
}

func TestRecordCounters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		record func(s *stats.Stats)
		check  func(s *stats.Stats) int
	}{
		{
			name:   "RecordInteraction",
			record: func(s *stats.Stats) { s.RecordInteraction() },
			check:  func(s *stats.Stats) int { return s.Interactions },
		},
		{
			name:   "RecordTurn",
			record: func(s *stats.Stats) { s.RecordTurn() },
			check:  func(s *stats.Stats) int { return s.Turns },
		},
		{
			name:   "RecordIteration",
			record: func(s *stats.Stats) { s.RecordIteration() },
			check:  func(s *stats.Stats) int { return s.Iterations },
		},
		{
			name:   "RecordToolError",
			record: func(s *stats.Stats) { s.RecordToolError("tool") },
			check:  func(s *stats.Stats) int { return s.Tools.Errors },
		},
		{
			name:   "RecordParseRetry",
			record: func(s *stats.Stats) { s.RecordParseRetry() },
			check:  func(s *stats.Stats) int { return s.ParseRetries },
		},
		{
			name:   "RecordCompaction",
			record: func(s *stats.Stats) { s.RecordCompaction() },
			check:  func(s *stats.Stats) int { return s.Compactions },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := stats.New()
			tt.record(s)
			tt.record(s)
			tt.record(s)

			if got := tt.check(s); got != 3 {
				t.Errorf("%s: counter = %d, want 3", tt.name, got)
			}
		})
	}
}

func TestRecordTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		calls   [][2]int
		wantIn  int
		wantOut int
	}{
		{
			name:    "single call",
			calls:   [][2]int{{100, 200}},
			wantIn:  100,
			wantOut: 200,
		},
		{
			name:    "multiple calls accumulate",
			calls:   [][2]int{{100, 200}, {50, 75}, {25, 25}},
			wantIn:  175,
			wantOut: 300,
		},
		{
			name:    "zero values",
			calls:   [][2]int{{0, 0}},
			wantIn:  0,
			wantOut: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := stats.New()

			for _, call := range tt.calls {
				s.RecordTokens(call[0], call[1])
			}

			if s.Tokens.In != tt.wantIn {
				t.Errorf("Tokens.In = %d, want %d", s.Tokens.In, tt.wantIn)
			}

			if s.Tokens.Out != tt.wantOut {
				t.Errorf("Tokens.Out = %d, want %d", s.Tokens.Out, tt.wantOut)
			}
		})
	}
}

func TestRecordToolCall(t *testing.T) {
	t.Parallel()

	s := stats.New()
	s.RecordToolCall("bash")
	s.RecordToolCall("bash")
	s.RecordToolCall("read")

	if s.Tools.Calls != 3 {
		t.Errorf("Tools.Calls = %d, want 3", s.Tools.Calls)
	}

	// Verify frequency tracking via TopTools
	top := s.TopTools(0)

	freq := make(map[string]int, len(top))
	for _, tc := range top {
		freq[tc.Name] = tc.Count
	}

	if freq["bash"] != 2 {
		t.Errorf("bash frequency = %d, want 2", freq["bash"])
	}

	if freq["read"] != 1 {
		t.Errorf("read frequency = %d, want 1", freq["read"])
	}
}

func TestTopTools(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		calls     map[string]int
		n         int
		wantLen   int
		wantFirst string
	}{
		{
			name:      "sorted descending",
			calls:     map[string]int{"alpha": 1, "beta": 5, "gamma": 3},
			n:         3,
			wantLen:   3,
			wantFirst: "beta",
		},
		{
			name:    "n limit respected",
			calls:   map[string]int{"alpha": 1, "beta": 5, "gamma": 3},
			n:       2,
			wantLen: 2,
			// top 2 are beta(5) and gamma(3)
			wantFirst: "beta",
		},
		{
			name:      "n larger than available",
			calls:     map[string]int{"alpha": 1, "beta": 5},
			n:         10,
			wantLen:   2,
			wantFirst: "beta",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := stats.New()

			for tool, count := range tt.calls {
				for range count {
					s.RecordToolCall(tool)
				}
			}

			got := s.TopTools(tt.n)

			if len(got) != tt.wantLen {
				t.Errorf("TopTools(%d) len = %d, want %d", tt.n, len(got), tt.wantLen)
			}

			if len(got) > 0 && got[0].Name != tt.wantFirst {
				t.Errorf("TopTools(%d)[0].Name = %q, want %q", tt.n, got[0].Name, tt.wantFirst)
			}

			// Verify descending order
			for i := 1; i < len(got); i++ {
				if got[i].Count > got[i-1].Count {
					t.Errorf("TopTools not sorted: got[%d].Count=%d > got[%d].Count=%d",
						i, got[i].Count, i-1, got[i-1].Count)
				}
			}
		})
	}
}

func TestTopToolsAll(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		n    int
	}{
		{name: "n=0 returns all", n: 0},
		{name: "n=-1 returns all", n: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := stats.New()
			s.RecordToolCall("alpha")
			s.RecordToolCall("beta")
			s.RecordToolCall("gamma")

			got := s.TopTools(tt.n)

			if len(got) != 3 {
				t.Errorf("TopTools(%d) len = %d, want 3 (all tools)", tt.n, len(got))
			}
		})
	}
}

func TestDisplay(t *testing.T) {
	t.Parallel()

	s := stats.New()
	s.RecordInteraction()
	s.RecordTurn()
	s.RecordToolCall("bash")
	s.RecordTokens(1000, 500)

	got := s.Display()

	if got == "" {
		t.Errorf("Display() returned empty string, want non-empty")
	}

	// Verify the output contains key section headers
	for _, want := range []string{"Session", "Tools"} {
		if !strings.Contains(got, want) {
			t.Errorf("Display() missing section %q", want)
		}
	}

	// Verify recorded values appear in the output.
	for _, want := range []string{"bash", "1 k", "500"} {
		if !strings.Contains(got, want) {
			t.Errorf("Display() missing value %q in output:\n%s", want, got)
		}
	}
}
