package wildcard_test

import (
	"testing"

	"github.com/idelchi/aura/pkg/wildcard"
)

func TestMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		pattern string
		want    bool
	}{
		{
			name:    "exact match",
			input:   "foobar",
			pattern: "foobar",
			want:    true,
		},
		{
			name:    "exact match does not accept partial",
			input:   "foobar",
			pattern: "foo",
			want:    false,
		},
		{
			name:    "star prefix matches any suffix",
			input:   "foobar",
			pattern: "*bar",
			want:    true,
		},
		{
			name:    "star prefix does not match wrong suffix",
			input:   "foobar",
			pattern: "*baz",
			want:    false,
		},
		{
			name:    "star suffix matches any prefix",
			input:   "foobar",
			pattern: "foo*",
			want:    true,
		},
		{
			name:    "star suffix does not match wrong prefix",
			input:   "foobar",
			pattern: "baz*",
			want:    false,
		},
		{
			name:    "star in middle matches substring",
			input:   "foobar",
			pattern: "foo*bar",
			want:    true,
		},
		{
			name:    "star in middle matches empty middle",
			input:   "foobar",
			pattern: "foobar*",
			want:    true,
		},
		{
			name:    "star alone matches any non-empty string",
			input:   "anything",
			pattern: "*",
			want:    true,
		},
		{
			name:    "star alone matches empty string",
			input:   "",
			pattern: "*",
			want:    true,
		},
		{
			name:    "empty pattern does not match non-empty name",
			input:   "foobar",
			pattern: "",
			want:    false,
		},
		{
			name:    "empty pattern matches empty name",
			input:   "",
			pattern: "",
			want:    true,
		},
		{
			name:    "dot is treated as literal not regex wildcard",
			input:   "fooXbar",
			pattern: "foo.bar",
			want:    false,
		},
		{
			name:    "dot matches literal dot",
			input:   "foo.bar",
			pattern: "foo.bar",
			want:    true,
		},
		{
			name:    "brackets are treated as literal characters",
			input:   "foo[bar]",
			pattern: "foo[bar]",
			want:    true,
		},
		{
			name:    "brackets do not act as regex character class",
			input:   "foob",
			pattern: "foo[bar]",
			want:    false,
		},
		{
			name:    "newline in name is not matched by star",
			input:   "foo\nbar",
			pattern: "foo*bar",
			want:    false,
		},
		{
			name:    "multiple stars work as multiple wildcards",
			input:   "foo-middle-bar",
			pattern: "foo*middle*bar",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := wildcard.Match(tt.input, tt.pattern)
			if got != tt.want {
				t.Errorf("Match(%q, %q) = %v, want %v", tt.input, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestMatchAny(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		patterns []string
		want     bool
	}{
		{
			name:     "no patterns returns false",
			input:    "foobar",
			patterns: []string{},
			want:     false,
		},
		{
			name:     "nil patterns returns false",
			input:    "foobar",
			patterns: nil,
			want:     false,
		},
		{
			name:     "single matching pattern returns true",
			input:    "foobar",
			patterns: []string{"foo*"},
			want:     true,
		},
		{
			name:     "single non-matching pattern returns false",
			input:    "foobar",
			patterns: []string{"baz*"},
			want:     false,
		},
		{
			name:     "one of several patterns matches returns true",
			input:    "foobar",
			patterns: []string{"baz*", "*bar", "qux"},
			want:     true,
		},
		{
			name:     "none of several patterns match returns false",
			input:    "foobar",
			patterns: []string{"baz*", "qux", "abc"},
			want:     false,
		},
		{
			name:     "multiple matching patterns still returns true",
			input:    "foobar",
			patterns: []string{"foo*", "*bar"},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := wildcard.MatchAny(tt.input, tt.patterns...)
			if got != tt.want {
				t.Errorf("MatchAny(%q, %v) = %v, want %v", tt.input, tt.patterns, got, tt.want)
			}
		})
	}
}

func TestMatchAll(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		patterns []string
		want     bool
	}{
		{
			name:     "no patterns returns true",
			input:    "foobar",
			patterns: []string{},
			want:     true,
		},
		{
			name:     "nil patterns returns true",
			input:    "foobar",
			patterns: nil,
			want:     true,
		},
		{
			name:     "single matching pattern returns true",
			input:    "foobar",
			patterns: []string{"foo*"},
			want:     true,
		},
		{
			name:     "single non-matching pattern returns false",
			input:    "foobar",
			patterns: []string{"baz*"},
			want:     false,
		},
		{
			name:     "all patterns match returns true",
			input:    "foobar",
			patterns: []string{"foo*", "*bar", "*ooba*"},
			want:     true,
		},
		{
			name:     "one pattern fails returns false",
			input:    "foobar",
			patterns: []string{"foo*", "*bar", "baz*"},
			want:     false,
		},
		{
			name:     "first pattern fails short circuits to false",
			input:    "foobar",
			patterns: []string{"baz*", "foo*"},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := wildcard.MatchAll(tt.input, tt.patterns...)
			if got != tt.want {
				t.Errorf("MatchAll(%q, %v) = %v, want %v", tt.input, tt.patterns, got, tt.want)
			}
		})
	}
}
