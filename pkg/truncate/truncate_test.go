package truncate_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/idelchi/aura/pkg/truncate"
)

func TestTruncate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		n           int
		wantSuffix  string // non-empty means we expect truncation
		wantExact   string // non-empty means we expect this exact string
		wantChanged bool
	}{
		{
			name:      "empty string unchanged",
			input:     "",
			n:         10,
			wantExact: "",
		},
		{
			name:      "string under limit unchanged",
			input:     "hello",
			n:         10,
			wantExact: "hello",
		},
		{
			name:      "string exactly at limit unchanged",
			input:     "hello",
			n:         5,
			wantExact: "hello",
		},
		{
			name:        "string over limit gets truncated with suffix",
			input:       "hello world",
			n:           5,
			wantChanged: true,
			wantSuffix:  "...",
		},
		{
			name:        "long string gets truncated with suffix",
			input:       strings.Repeat("a", 1000),
			n:           100,
			wantChanged: true,
			wantSuffix:  "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := truncate.Truncate(tt.input, tt.n)

			if tt.wantExact != "" || (!tt.wantChanged && tt.wantSuffix == "") {
				if got != tt.wantExact {
					t.Errorf("Truncate(%q, %d) = %q, want %q", tt.input, tt.n, got, tt.wantExact)
				}

				return
			}

			if tt.wantSuffix != "" {
				if !strings.HasSuffix(got, tt.wantSuffix) {
					t.Errorf("Truncate(%q, %d) = %q, want suffix %q", tt.input, tt.n, got, tt.wantSuffix)
				}

				if got == tt.input {
					t.Errorf("Truncate(%q, %d) = unchanged input, expected truncation", tt.input, tt.n)
				}
			}
		})
	}
}

func TestFormatArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       map[string]any
		maxLen     int
		want       string
		wantSuffix string // set when result is truncated to "..."
	}{
		{
			name:   "empty map returns empty string",
			args:   map[string]any{},
			maxLen: 100,
			want:   "",
		},
		{
			name:   "nil map returns empty string",
			args:   nil,
			maxLen: 100,
			want:   "",
		},
		{
			name:   "single key value pair",
			args:   map[string]any{"file": "foo.go"},
			maxLen: 100,
			want:   `file=foo.go`,
		},
		{
			name:   "multiple keys are sorted alphabetically",
			args:   map[string]any{"zoo": "last", "alpha": "first", "middle": 42},
			maxLen: 200,
			want:   "alpha=first middle=42 zoo=last",
		},
		{
			name:       "result exceeding maxLen is replaced with ellipsis",
			args:       map[string]any{"key": "a very long value that definitely exceeds the limit"},
			maxLen:     10,
			wantSuffix: "...",
		},
		{
			name:       "result exactly over maxLen uses ellipsis",
			args:       map[string]any{"k": "vvvvvvvvvv"},
			maxLen:     5,
			wantSuffix: "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := truncate.FormatArgs(tt.args, tt.maxLen)

			if tt.wantSuffix != "" {
				if !strings.HasSuffix(got, tt.wantSuffix) {
					t.Errorf("FormatArgs(%v, %d) = %q, want suffix %q", tt.args, tt.maxLen, got, tt.wantSuffix)
				}

				if len(got) > tt.maxLen {
					t.Errorf(
						"FormatArgs(%v, %d) = %q (len %d), exceeds maxLen %d",
						tt.args,
						tt.maxLen,
						got,
						len(got),
						tt.maxLen,
					)
				}

				return
			}

			if got != tt.want {
				t.Errorf("FormatArgs(%v, %d) = %q, want %q", tt.args, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestMapToJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		m    map[string]any
	}{
		{
			name: "empty map produces valid JSON",
			m:    map[string]any{},
		},
		{
			name: "nil map produces valid JSON",
			m:    nil,
		},
		{
			name: "string values produce valid JSON",
			m:    map[string]any{"key": "value", "name": "test"},
		},
		{
			name: "numeric values produce valid JSON",
			m:    map[string]any{"count": 42, "ratio": 3.14},
		},
		{
			name: "mixed value types produce valid JSON",
			m:    map[string]any{"str": "hello", "num": 1, "flag": true},
		},
		{
			name: "nested map produces valid JSON",
			m:    map[string]any{"outer": map[string]any{"inner": "value"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := truncate.MapToJSON(tt.m)

			if !json.Valid([]byte(got)) {
				t.Errorf("MapToJSON(%v) = %q, want valid JSON", tt.m, got)
			}

			// Round-trip: unmarshal and check keys match.
			var decoded map[string]any
			if err := json.Unmarshal([]byte(got), &decoded); err != nil {
				t.Fatalf("MapToJSON(%v) produced JSON that could not be unmarshalled: %v", tt.m, err)
			}

			if len(decoded) != len(tt.m) {
				t.Errorf("MapToJSON(%v) round-trip has %d keys, want %d", tt.m, len(decoded), len(tt.m))
			}
		})
	}
}
