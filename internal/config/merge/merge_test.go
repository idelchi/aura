package merge_test

import (
	"testing"

	"github.com/idelchi/aura/internal/config/merge"
)

type stringSliceStruct struct {
	Tags []string
}

type intSliceStruct struct {
	Retries []int
}

type stringMapStruct struct {
	Labels map[string]string
}

type anyMapStruct struct {
	Config map[string]any
}

type pointerStruct struct {
	Enabled *bool
	Count   *int
}

func TestMergeStringSliceNilInherits(t *testing.T) {
	t.Parallel()

	dst := stringSliceStruct{Tags: []string{"a", "b"}}
	src := stringSliceStruct{Tags: nil}

	if err := merge.Merge(&dst, src); err != nil {
		t.Fatal(err)
	}

	if len(dst.Tags) != 2 || dst.Tags[0] != "a" {
		t.Errorf("Tags = %v, want [a b]", dst.Tags)
	}
}

func TestMergeStringSliceEmptyClears(t *testing.T) {
	t.Parallel()

	dst := stringSliceStruct{Tags: []string{"a", "b"}}
	src := stringSliceStruct{Tags: []string{}}

	if err := merge.Merge(&dst, src); err != nil {
		t.Fatal(err)
	}

	if len(dst.Tags) != 0 {
		t.Errorf("Tags = %v, want []", dst.Tags)
	}
}

func TestMergeStringSlicePopulatedReplaces(t *testing.T) {
	t.Parallel()

	dst := stringSliceStruct{Tags: []string{"a", "b"}}
	src := stringSliceStruct{Tags: []string{"x"}}

	if err := merge.Merge(&dst, src); err != nil {
		t.Fatal(err)
	}

	if len(dst.Tags) != 1 || dst.Tags[0] != "x" {
		t.Errorf("Tags = %v, want [x]", dst.Tags)
	}
}

func TestMergeIntSliceNilInherits(t *testing.T) {
	t.Parallel()

	dst := intSliceStruct{Retries: []int{100, 50, 0}}
	src := intSliceStruct{Retries: nil}

	if err := merge.Merge(&dst, src); err != nil {
		t.Fatal(err)
	}

	if len(dst.Retries) != 3 || dst.Retries[0] != 100 {
		t.Errorf("Retries = %v, want [100 50 0]", dst.Retries)
	}
}

func TestMergeIntSliceEmptyClears(t *testing.T) {
	t.Parallel()

	dst := intSliceStruct{Retries: []int{100, 50, 0}}
	src := intSliceStruct{Retries: []int{}}

	if err := merge.Merge(&dst, src); err != nil {
		t.Fatal(err)
	}

	if len(dst.Retries) != 0 {
		t.Errorf("Retries = %v, want []", dst.Retries)
	}
}

func TestMergeStringMapNilInherits(t *testing.T) {
	t.Parallel()

	dst := stringMapStruct{Labels: map[string]string{"a": "1"}}
	src := stringMapStruct{Labels: nil}

	if err := merge.Merge(&dst, src); err != nil {
		t.Fatal(err)
	}

	if len(dst.Labels) != 1 || dst.Labels["a"] != "1" {
		t.Errorf("Labels = %v, want map[a:1]", dst.Labels)
	}
}

func TestMergeStringMapEmptyClears(t *testing.T) {
	t.Parallel()

	dst := stringMapStruct{Labels: map[string]string{"a": "1"}}
	src := stringMapStruct{Labels: map[string]string{}}

	if err := merge.Merge(&dst, src); err != nil {
		t.Fatal(err)
	}

	if len(dst.Labels) != 0 {
		t.Errorf("Labels = %v, want empty map", dst.Labels)
	}
}

func TestMergeAnyMapNilInherits(t *testing.T) {
	t.Parallel()

	dst := anyMapStruct{Config: map[string]any{"k": "v"}}
	src := anyMapStruct{Config: nil}

	if err := merge.Merge(&dst, src); err != nil {
		t.Fatal(err)
	}

	if len(dst.Config) != 1 || dst.Config["k"] != "v" {
		t.Errorf("Config = %v, want map[k:v]", dst.Config)
	}
}

func TestMergeAnyMapEmptyClears(t *testing.T) {
	t.Parallel()

	dst := anyMapStruct{Config: map[string]any{"k": "v"}}
	src := anyMapStruct{Config: map[string]any{}}

	if err := merge.Merge(&dst, src); err != nil {
		t.Fatal(err)
	}

	if len(dst.Config) != 0 {
		t.Errorf("Config = %v, want empty map", dst.Config)
	}
}

func TestMergeAnyMapPopulatedReplaces(t *testing.T) {
	t.Parallel()

	dst := anyMapStruct{Config: map[string]any{"a": 1, "b": 2}}
	src := anyMapStruct{Config: map[string]any{"c": 3}}

	if err := merge.Merge(&dst, src); err != nil {
		t.Fatal(err)
	}

	if len(dst.Config) != 1 || dst.Config["c"] != 3 {
		t.Errorf("Config = %v, want map[c:3]", dst.Config)
	}
}

func TestMergePointerNilInherits(t *testing.T) {
	t.Parallel()

	val := true
	dst := pointerStruct{Enabled: &val}
	src := pointerStruct{Enabled: nil}

	if err := merge.Merge(&dst, src); err != nil {
		t.Fatal(err)
	}

	if dst.Enabled == nil || !*dst.Enabled {
		t.Errorf("Enabled = %v, want &true", dst.Enabled)
	}
}

func TestMergePointerNonNilOverrides(t *testing.T) {
	t.Parallel()

	val := true
	override := false
	dst := pointerStruct{Enabled: &val}
	src := pointerStruct{Enabled: &override}

	if err := merge.Merge(&dst, src); err != nil {
		t.Fatal(err)
	}

	if dst.Enabled == nil || *dst.Enabled {
		t.Errorf("Enabled = %v, want &false", dst.Enabled)
	}
}

// TestMergeStructPointer verifies replacement, inheritance and explicit empty overrides.
func TestMergeStructPointer(t *testing.T) {
	t.Parallel()

	type options struct {
		Command string
		Retries int
	}
	type config struct {
		Options *options
	}

	for _, test := range []struct {
		name string
		src  *options
		want options
	}{
		{name: "nil inherits", want: options{Command: "parent", Retries: 2}},
		{name: "non-nil replaces", src: &options{Command: "child"}, want: options{Command: "child"}},
		{name: "empty replaces", src: &options{}, want: options{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			parent := options{Command: "parent", Retries: 2}
			dst := config{Options: &parent}
			if err := merge.Merge(&dst, config{Options: test.src}); err != nil {
				t.Fatal(err)
			}
			if dst.Options == nil || *dst.Options != test.want {
				t.Errorf("Options = %+v, want %+v", dst.Options, test.want)
			}
			if parent != (options{Command: "parent", Retries: 2}) {
				t.Errorf("merge changed parent: %+v", parent)
			}
		})
	}
}
