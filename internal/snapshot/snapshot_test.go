package snapshot

import (
	"testing"
)

func TestToSet(t *testing.T) {
	t.Parallel()

	got := toSet([]string{"a", "", "b", "a"})
	want := map[string]bool{"a": true, "b": true}

	if len(got) != len(want) {
		t.Fatalf("toSet returned map of len %d; want %d", len(got), len(want))
	}

	for k, v := range want {
		if got[k] != v {
			t.Errorf("toSet result missing key %q", k)
		}
	}

	if got[""] {
		t.Errorf("toSet result contains empty string key; want it excluded")
	}
}

func TestToSetEmpty(t *testing.T) {
	t.Parallel()

	got := toSet([]string{})
	if len(got) != 0 {
		t.Errorf("toSet([]string{}) returned map of len %d; want 0", len(got))
	}
}
