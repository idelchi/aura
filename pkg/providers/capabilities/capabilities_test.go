package capabilities_test

import (
	"testing"

	"github.com/idelchi/aura/pkg/providers/capabilities"
)

func TestAddHas(t *testing.T) {
	t.Parallel()

	t.Run("add makes Has return true", func(t *testing.T) {
		t.Parallel()

		var cs capabilities.Capabilities
		cs.Add(capabilities.Vision)

		if !cs.Has(capabilities.Vision) {
			t.Errorf("Has(Vision) = false after Add, want true")
		}
	})

	t.Run("Has returns false for missing capability", func(t *testing.T) {
		t.Parallel()

		var cs capabilities.Capabilities
		cs.Add(capabilities.Vision)

		if cs.Has(capabilities.Tools) {
			t.Errorf("Has(Tools) = true for capability not added, want false")
		}
	})

	t.Run("duplicate add is no-op", func(t *testing.T) {
		t.Parallel()

		var cs capabilities.Capabilities
		cs.Add(capabilities.Thinking)
		cs.Add(capabilities.Thinking)

		count := 0

		for _, c := range cs {
			if c == capabilities.Thinking {
				count++
			}
		}

		if count != 1 {
			t.Errorf("duplicate Add resulted in %d entries, want 1", count)
		}
	})
}

func TestPredicates(t *testing.T) {
	t.Parallel()

	// allPredicates maps each capability to its predicate method and a label.
	type predicateFn struct {
		cap       capabilities.Capability
		predicate func(capabilities.Capabilities) bool
		label     string
	}

	all := []predicateFn{
		{capabilities.Thinking, capabilities.Capabilities.Thinking, "Thinking"},
		{capabilities.ThinkingLevels, capabilities.Capabilities.ThinkingLevels, "ThinkingLevels"},
		{capabilities.Tools, capabilities.Capabilities.Tools, "Tools"},
		{capabilities.Embedding, capabilities.Capabilities.Embedding, "Embedding"},
		{capabilities.Reranking, capabilities.Capabilities.Reranking, "Reranking"},
		{capabilities.Vision, capabilities.Capabilities.Vision, "Vision"},
		{capabilities.ContextOverride, capabilities.Capabilities.ContextOverride, "ContextOverride"},
	}

	for _, subject := range all {
		t.Run(subject.label+" predicate true when added", func(t *testing.T) {
			t.Parallel()

			var cs capabilities.Capabilities
			cs.Add(subject.cap)

			if !subject.predicate(cs) {
				t.Errorf("%s() = false after adding %s, want true", subject.label, subject.label)
			}
		})

		t.Run(subject.label+" predicate false for others", func(t *testing.T) {
			t.Parallel()

			var cs capabilities.Capabilities
			// Add all capabilities except the subject.
			for _, other := range all {
				if other.cap != subject.cap {
					cs.Add(other.cap)
				}
			}

			if subject.predicate(cs) {
				t.Errorf("%s() = true when only other capabilities added, want false", subject.label)
			}
		})
	}
}

func TestMap(t *testing.T) {
	t.Parallel()

	t.Run("map always has 7 keys", func(t *testing.T) {
		t.Parallel()

		var cs capabilities.Capabilities

		m := cs.Map()
		if len(m) != 7 {
			t.Errorf("Map() returned %d keys, want 7", len(m))
		}
	})

	t.Run("map values match Has for added capabilities", func(t *testing.T) {
		t.Parallel()

		var cs capabilities.Capabilities
		cs.Add(capabilities.Vision)
		cs.Add(capabilities.Tools)
		cs.Add(capabilities.Thinking)

		m := cs.Map()

		// Verify the 7 fixed keys exist.
		expectedKeys := []string{
			"thinking",
			"thinking_levels",
			"tools",
			"vision",
			"embedding",
			"reranking",
			"context_override",
		}
		for _, k := range expectedKeys {
			if _, ok := m[k]; !ok {
				t.Errorf("Map() missing key %q", k)
			}
		}

		// Added capabilities should be true.
		if !m["vision"] {
			t.Errorf("Map()[\"vision\"] = false, want true")
		}

		if !m["tools"] {
			t.Errorf("Map()[\"tools\"] = false, want true")
		}

		if !m["thinking"] {
			t.Errorf("Map()[\"thinking\"] = false, want true")
		}

		// Non-added capabilities should be false.
		if m["embedding"] {
			t.Errorf("Map()[\"embedding\"] = true, want false")
		}

		if m["reranking"] {
			t.Errorf("Map()[\"reranking\"] = true, want false")
		}

		if m["thinking_levels"] {
			t.Errorf("Map()[\"thinking_levels\"] = true, want false")
		}

		if m["context_override"] {
			t.Errorf("Map()[\"context_override\"] = true, want false")
		}
	})
}
