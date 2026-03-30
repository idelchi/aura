package spintext_test

import (
	"slices"
	"testing"

	"github.com/idelchi/aura/internal/spintext"
)

func TestDefault(t *testing.T) {
	t.Parallel()

	s := spintext.Default()

	if s == nil {
		t.Fatal("Default() returned nil")
	}

	if len(*s) == 0 {
		t.Errorf("Default() returned empty SpinText, want non-empty slice")
	}

	if len(*s) != 49 {
		t.Errorf("Default() len = %d, want 49", len(*s))
	}
}

func TestRandom(t *testing.T) {
	t.Parallel()

	s := spintext.Default()
	got := s.Random()

	if got == "" {
		t.Errorf("Random() returned empty string, want non-empty")
	}

	if !slices.Contains([]string(*s), got) {
		t.Errorf("Random() = %q, not found in Default() slice", got)
	}
}

func TestRandomEmpty(t *testing.T) {
	t.Parallel()

	s := spintext.SpinText{}
	got := s.Random()

	const want = "Working..."
	if got != want {
		t.Errorf("Random() on empty SpinText = %q, want %q", got, want)
	}
}
