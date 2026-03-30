package indexer

import (
	"strings"
	"testing"
)

func TestResultDisplay(t *testing.T) {
	t.Parallel()

	r := Result{Path: "foo.go", Content: "some code", StartLine: 10, EndLine: 20, Similarity: 0.95}
	out := r.Display()

	if !strings.Contains(out, "foo.go") {
		t.Errorf("Display() = %q, want it to contain %q", out, "foo.go")
	}

	if !strings.Contains(out, "10") {
		t.Errorf("Display() = %q, want it to contain %q", out, "10")
	}

	if !strings.Contains(out, "20") {
		t.Errorf("Display() = %q, want it to contain %q", out, "20")
	}
}

func TestResultsDisplay(t *testing.T) {
	t.Parallel()

	rs := Results{
		{Path: "alpha.go", Content: "a", StartLine: 1, EndLine: 5, Similarity: 0.9},
		{Path: "beta.go", Content: "b", StartLine: 6, EndLine: 10, Similarity: 0.8},
	}
	out := rs.Display()

	if !strings.Contains(out, "alpha.go") {
		t.Errorf("Display() = %q, want it to contain %q", out, "alpha.go")
	}

	if !strings.Contains(out, "beta.go") {
		t.Errorf("Display() = %q, want it to contain %q", out, "beta.go")
	}
}

func TestDisplayWithRerankedNoReranker(t *testing.T) {
	t.Parallel()

	rs := Results{
		{Path: "main.go", Content: "x", StartLine: 1, EndLine: 3, Similarity: 0.7},
	}
	out := rs.DisplayWithReranked(nil, false)

	if strings.Contains(strings.ToLower(out), "reranked") {
		t.Errorf("DisplayWithReranked(nil, false) = %q, should not contain reranked section", out)
	}

	if !strings.Contains(out, "main.go") {
		t.Errorf("DisplayWithReranked(nil, false) = %q, want it to contain %q", out, "main.go")
	}
}

func TestDisplayWithRerankedWithReranker(t *testing.T) {
	t.Parallel()

	rs := Results{
		{Path: "orig.go", Content: "o", StartLine: 1, EndLine: 2, Similarity: 0.6},
	}
	reranked := Results{
		{Path: "rerank.go", Content: "r", StartLine: 3, EndLine: 4, Similarity: 0.9},
	}
	out := rs.DisplayWithReranked(reranked, true)

	if !strings.Contains(out, "Unsorted:") {
		t.Errorf("DisplayWithReranked output = %q, want it to contain %q", out, "Unsorted:")
	}

	if !strings.Contains(out, "Reranked:") {
		t.Errorf("DisplayWithReranked output = %q, want it to contain %q", out, "Reranked:")
	}

	if !strings.Contains(out, "orig.go") {
		t.Errorf("DisplayWithReranked output = %q, want it to contain orig.go", out)
	}

	if !strings.Contains(out, "rerank.go") {
		t.Errorf("DisplayWithReranked output = %q, want it to contain rerank.go", out)
	}
}

func TestDedupRemovesDuplicates(t *testing.T) {
	t.Parallel()

	// dedup keeps the first occurrence per path; put the higher-similarity entry first.
	results := Results{
		{Path: "dup.go", Content: "a", StartLine: 1, EndLine: 5, Similarity: 0.9},
		{Path: "dup.go", Content: "b", StartLine: 6, EndLine: 10, Similarity: 0.5},
		{Path: "unique.go", Content: "c", StartLine: 1, EndLine: 3, Similarity: 0.7},
	}
	got := dedup(results, 10)

	if len(got) != 2 {
		t.Fatalf("dedup() len = %d, want 2", len(got))
	}

	var dupResult Result

	for _, r := range got {
		if r.Path == "dup.go" {
			dupResult = r
		}
	}

	if dupResult.Path == "" {
		t.Fatalf("dedup() result missing dup.go entry")
	}

	if dupResult.Similarity != 0.9 {
		t.Errorf("dedup() kept dup.go with Similarity = %v, want 0.9 (higher score)", dupResult.Similarity)
	}
}

func TestDedupRespectsLimit(t *testing.T) {
	t.Parallel()

	results := Results{
		{Path: "a.go", Similarity: 0.9},
		{Path: "b.go", Similarity: 0.8},
		{Path: "c.go", Similarity: 0.7},
		{Path: "d.go", Similarity: 0.6},
		{Path: "e.go", Similarity: 0.5},
	}
	got := dedup(results, 2)

	if len(got) != 2 {
		t.Errorf("dedup() len = %d, want 2", len(got))
	}
}

func TestDedupEmpty(t *testing.T) {
	t.Parallel()

	got := dedup(Results{}, 10)

	if len(got) != 0 {
		t.Errorf("dedup(empty) len = %d, want 0", len(got))
	}
}
