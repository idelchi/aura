package rerank_test

import (
	"testing"

	"github.com/idelchi/aura/pkg/llm/rerank"
)

func TestIndices(t *testing.T) {
	t.Parallel()

	results := rerank.Results{
		{Index: 3},
		{Index: 1},
		{Index: 0},
	}

	got := results.Indices()
	want := []int{3, 1, 0}

	if len(got) != len(want) {
		t.Fatalf("Indices() len = %d, want %d", len(got), len(want))
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Indices()[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestIndicesEmpty(t *testing.T) {
	t.Parallel()

	var results rerank.Results

	got := results.Indices()

	if len(got) != 0 {
		t.Errorf("Indices() on empty Results len = %d, want 0", len(got))
	}
}
