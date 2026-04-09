package config_test

import (
	"testing"

	"github.com/idelchi/aura/internal/config"
)

func TestFeaturesMergeFromOverride(t *testing.T) {
	t.Parallel()

	base := config.Features{
		Compaction: config.Compaction{Threshold: 80},
	}
	overlay := config.Features{
		Compaction: config.Compaction{Threshold: 50},
	}

	if err := base.MergeFrom(overlay); err != nil {
		t.Fatalf("MergeFrom() error = %v", err)
	}

	if base.Compaction.Threshold != 50 {
		t.Errorf("Compaction.Threshold = %v, want 50", base.Compaction.Threshold)
	}
}

func TestFeaturesMergeFromZeroNoOverride(t *testing.T) {
	t.Parallel()

	base := config.Features{
		Compaction: config.Compaction{Threshold: 80},
	}
	overlay := config.Features{} // zero value — should not overwrite

	if err := base.MergeFrom(overlay); err != nil {
		t.Fatalf("MergeFrom() error = %v", err)
	}

	if base.Compaction.Threshold != 80 {
		t.Errorf("Compaction.Threshold = %v, want 80 (zero overlay must not overwrite)", base.Compaction.Threshold)
	}
}

func TestFeaturesMergeFromNested(t *testing.T) {
	t.Parallel()

	base := config.Features{
		ToolExecution: config.ToolExecution{MaxSteps: 50},
	}
	overlay := config.Features{
		ToolExecution: config.ToolExecution{MaxSteps: 100},
	}

	if err := base.MergeFrom(overlay); err != nil {
		t.Fatalf("MergeFrom() error = %v", err)
	}

	if base.ToolExecution.MaxSteps != 100 {
		t.Errorf("ToolExecution.MaxSteps = %v, want 100", base.ToolExecution.MaxSteps)
	}
}

func TestFeaturesMergeFromPreservesUnchangedFields(t *testing.T) {
	t.Parallel()

	base := config.Features{
		Compaction: config.Compaction{
			Threshold:        80,
			KeepLastMessages: 10,
		},
	}
	// Overlay only sets Threshold; KeepLastMessages is zero → must not overwrite.
	overlay := config.Features{
		Compaction: config.Compaction{Threshold: 60},
	}

	if err := base.MergeFrom(overlay); err != nil {
		t.Fatalf("MergeFrom() error = %v", err)
	}

	if base.Compaction.Threshold != 60 {
		t.Errorf("Compaction.Threshold = %v, want 60", base.Compaction.Threshold)
	}

	if base.Compaction.KeepLastMessages != 10 {
		t.Errorf("Compaction.KeepLastMessages = %v, want 10 (must be preserved)", base.Compaction.KeepLastMessages)
	}
}

func TestFeaturesMergeFromMultipleFields(t *testing.T) {
	t.Parallel()

	base := config.Features{
		Compaction:    config.Compaction{Threshold: 80},
		ToolExecution: config.ToolExecution{MaxSteps: 50},
	}
	overlay := config.Features{
		ToolExecution: config.ToolExecution{MaxSteps: 200},
		// Compaction is zero — base value must be preserved.
	}

	if err := base.MergeFrom(overlay); err != nil {
		t.Fatalf("MergeFrom() error = %v", err)
	}

	if base.Compaction.Threshold != 80 {
		t.Errorf("Compaction.Threshold = %v, want 80", base.Compaction.Threshold)
	}

	if base.ToolExecution.MaxSteps != 200 {
		t.Errorf("ToolExecution.MaxSteps = %v, want 200", base.ToolExecution.MaxSteps)
	}
}

func TestFeaturesMergeFromEmptySliceClears(t *testing.T) {
	t.Parallel()

	base := config.Features{
		ToolExecution: config.ToolExecution{Disabled: []string{"Bash", "Patch"}},
	}
	overlay := config.Features{
		ToolExecution: config.ToolExecution{Disabled: []string{}}, // explicitly empty — should clear
	}

	if err := base.MergeFrom(overlay); err != nil {
		t.Fatalf("MergeFrom() error = %v", err)
	}

	if len(base.ToolExecution.Disabled) != 0 {
		t.Errorf("ToolExecution.Disabled = %v, want [] (empty slice should clear parent)", base.ToolExecution.Disabled)
	}
}

func TestFeaturesMergeFromNilSliceInherits(t *testing.T) {
	t.Parallel()

	base := config.Features{
		ToolExecution: config.ToolExecution{Disabled: []string{"Bash", "Patch"}},
	}
	overlay := config.Features{} // nil Disabled — should inherit

	if err := base.MergeFrom(overlay); err != nil {
		t.Fatalf("MergeFrom() error = %v", err)
	}

	if len(base.ToolExecution.Disabled) != 2 {
		t.Errorf("ToolExecution.Disabled = %v, want [Bash Patch] (nil should inherit)", base.ToolExecution.Disabled)
	}
}

func TestSandboxEffectiveRestrictions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		base   config.Restrictions
		extra  config.Restrictions
		wantRO []string
		wantRW []string
	}{
		{
			name:   "concatenates disjoint sets",
			base:   config.Restrictions{ReadOnly: []string{"/a"}, ReadWrite: []string{"/b"}},
			extra:  config.Restrictions{ReadOnly: []string{"/c"}, ReadWrite: []string{"/d"}},
			wantRO: []string{"/a", "/c"},
			wantRW: []string{"/b", "/d"},
		},
		{
			name:   "empty extra preserves base",
			base:   config.Restrictions{ReadOnly: []string{"/a"}, ReadWrite: []string{"/b"}},
			extra:  config.Restrictions{},
			wantRO: []string{"/a"},
			wantRW: []string{"/b"},
		},
		{
			name:   "empty base with extra returns extra",
			base:   config.Restrictions{},
			extra:  config.Restrictions{ReadOnly: []string{"/c"}, ReadWrite: []string{"/d"}},
			wantRO: []string{"/c"},
			wantRW: []string{"/d"},
		},
		{
			name:   "both empty produces nil",
			base:   config.Restrictions{},
			extra:  config.Restrictions{},
			wantRO: nil,
			wantRW: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sf := config.SandboxFeature{
				Restrictions: tc.base,
				Extra:        tc.extra,
			}
			got := sf.EffectiveRestrictions()

			checkSlice := func(label string, got, want []string) {
				t.Helper()

				if len(got) != len(want) {
					t.Errorf("%s: len = %d, want %d; got %v", label, len(got), len(want), got)

					return
				}

				for i, v := range want {
					if got[i] != v {
						t.Errorf("%s[%d] = %q, want %q", label, i, got[i], v)
					}
				}
			}

			checkSlice("ReadOnly", got.ReadOnly, tc.wantRO)
			checkSlice("ReadWrite", got.ReadWrite, tc.wantRW)
		})
	}
}
