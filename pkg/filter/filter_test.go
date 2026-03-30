package filter_test

import (
	"testing"

	"github.com/idelchi/aura/pkg/filter"
)

// --- Test structs ---

type nested struct {
	Inner innerStruct `yaml:"inner"`
}

type innerStruct struct {
	Name  string `yaml:"name"`
	Value int    `yaml:"value"`
}

type deep struct {
	Level1 struct {
		Level2 struct {
			Level3 string `yaml:"level3"`
		} `yaml:"level2"`
	} `yaml:"level1"`
}

type withSlice struct {
	Tags []string `yaml:"tags"`
}

type withMap struct {
	Labels map[string]string `yaml:"labels"`
}

type withPointer struct {
	Name     string `yaml:"name"`
	Disabled *bool  `yaml:"disabled"`
}

type withMixed struct {
	Name    string         `yaml:"name"`
	Count   int            `yaml:"count"`
	Enabled bool           `yaml:"enabled"`
	Float   float64        `yaml:"float"`
	Extra   map[string]any `yaml:"extra"`
}

type withOmitempty struct {
	Name string `yaml:"name"`
	Tag  string `yaml:"tag,omitempty"`
}

// --- Tests ---

func TestMatch_EmptyFilters(t *testing.T) {
	t.Parallel()

	ok, err := filter.Match(nested{Inner: innerStruct{Name: "x"}}, nil)
	if err != nil || !ok {
		t.Fatalf("empty filters should always match: ok=%v err=%v", ok, err)
	}

	ok, err = filter.Match(nested{}, []string{})
	if err != nil || !ok {
		t.Fatalf("empty slice should always match: ok=%v err=%v", ok, err)
	}
}

func TestMatch_TopLevelField(t *testing.T) {
	t.Parallel()

	v := innerStruct{Name: "hello", Value: 42}

	ok, err := filter.Match(v, []string{"name=hello"})
	if err != nil || !ok {
		t.Fatalf("expected match: ok=%v err=%v", ok, err)
	}

	ok, err = filter.Match(v, []string{"name=goodbye"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ok {
		t.Fatal("expected no match")
	}
}

func TestMatch_NestedDotPath(t *testing.T) {
	t.Parallel()

	v := nested{Inner: innerStruct{Name: "deep", Value: 99}}

	ok, err := filter.Match(v, []string{"inner.name=deep"})
	if err != nil || !ok {
		t.Fatalf("nested match failed: ok=%v err=%v", ok, err)
	}

	ok, err = filter.Match(v, []string{"inner.value=99"})
	if err != nil || !ok {
		t.Fatalf("nested int match failed: ok=%v err=%v", ok, err)
	}
}

func TestMatch_ThreeLevelDeep(t *testing.T) {
	t.Parallel()

	v := deep{}

	v.Level1.Level2.Level3 = "found"

	ok, err := filter.Match(v, []string{"level1.level2.level3=found"})
	if err != nil || !ok {
		t.Fatalf("3-level deep match failed: ok=%v err=%v", ok, err)
	}
}

func TestMatch_CaseInsensitive(t *testing.T) {
	t.Parallel()

	v := innerStruct{Name: "Hello"}

	ok, err := filter.Match(v, []string{"name=hello"})
	if err != nil || !ok {
		t.Fatalf("case-insensitive match failed: ok=%v err=%v", ok, err)
	}

	ok, err = filter.Match(v, []string{"name=HELLO"})
	if err != nil || !ok {
		t.Fatalf("uppercase match failed: ok=%v err=%v", ok, err)
	}
}

func TestMatch_MultipleFilters_AllMustMatch(t *testing.T) {
	t.Parallel()

	v := innerStruct{Name: "foo", Value: 10}

	// Both match.
	ok, err := filter.Match(v, []string{"name=foo", "value=10"})
	if err != nil || !ok {
		t.Fatalf("both-match failed: ok=%v err=%v", ok, err)
	}

	// First matches, second doesn't.
	ok, err = filter.Match(v, []string{"name=foo", "value=99"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ok {
		t.Fatal("expected no match when second filter fails")
	}
}

func TestMatch_MissingKey_NoMatch(t *testing.T) {
	t.Parallel()

	v := innerStruct{Name: "x"}

	ok, err := filter.Match(v, []string{"nonexistent=x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ok {
		t.Fatal("expected no match for missing key")
	}
}

func TestMatch_MissingNestedKey_NoMatch(t *testing.T) {
	t.Parallel()

	v := nested{Inner: innerStruct{Name: "x"}}

	ok, err := filter.Match(v, []string{"inner.missing=x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ok {
		t.Fatal("expected no match for missing nested key")
	}
}

func TestMatch_PathThroughScalar_NoMatch(t *testing.T) {
	t.Parallel()

	v := innerStruct{Name: "x"}

	// name is a string, can't traverse further
	ok, err := filter.Match(v, []string{"name.deeper=x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ok {
		t.Fatal("expected no match when path goes through scalar")
	}
}

func TestMatch_BooleanField(t *testing.T) {
	t.Parallel()

	v := withMixed{Enabled: true}

	ok, err := filter.Match(v, []string{"enabled=true"})
	if err != nil || !ok {
		t.Fatalf("bool true match failed: ok=%v err=%v", ok, err)
	}

	ok, err = filter.Match(v, []string{"enabled=false"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ok {
		t.Fatal("expected no match for bool mismatch")
	}
}

func TestMatch_FloatField(t *testing.T) {
	t.Parallel()

	v := withMixed{Float: 3.14}

	ok, err := filter.Match(v, []string{"float=3.14"})
	if err != nil || !ok {
		t.Fatalf("float match failed: ok=%v err=%v", ok, err)
	}
}

func TestMatch_IntField(t *testing.T) {
	t.Parallel()

	v := withMixed{Count: 42}

	ok, err := filter.Match(v, []string{"count=42"})
	if err != nil || !ok {
		t.Fatalf("int match failed: ok=%v err=%v", ok, err)
	}
}

func TestMatch_NilPointer(t *testing.T) {
	t.Parallel()

	v := withPointer{Name: "test", Disabled: nil}

	ok, err := filter.Match(v, []string{"disabled=<nil>"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// nil pointer marshals to null in YAML, which becomes nil in map.
	// fmt.Sprint(nil) = "<nil>", so this should match.
	if !ok {
		t.Fatal("expected match for nil pointer with <nil> value")
	}
}

func TestMatch_NonNilPointer(t *testing.T) {
	t.Parallel()

	b := true
	v := withPointer{Name: "test", Disabled: &b}

	ok, err := filter.Match(v, []string{"disabled=true"})
	if err != nil || !ok {
		t.Fatalf("non-nil pointer match failed: ok=%v err=%v", ok, err)
	}
}

func TestMatch_SliceField(t *testing.T) {
	t.Parallel()

	v := withSlice{Tags: []string{"a", "b", "c"}}

	// Slice stringified as "[a b c]" by fmt.Sprint
	ok, err := filter.Match(v, []string{"tags=[a b c]"})
	if err != nil || !ok {
		t.Fatalf("slice match failed: ok=%v err=%v", ok, err)
	}
}

func TestMatch_MapField(t *testing.T) {
	t.Parallel()

	v := withMap{Labels: map[string]string{"env": "prod"}}

	ok, err := filter.Match(v, []string{"labels.env=prod"})
	if err != nil || !ok {
		t.Fatalf("map dot-path match failed: ok=%v err=%v", ok, err)
	}
}

func TestMatch_MapNestedInStruct(t *testing.T) {
	t.Parallel()

	v := withMixed{
		Name:  "test",
		Extra: map[string]any{"nested": map[string]any{"key": "val"}},
	}

	ok, err := filter.Match(v, []string{"extra.nested.key=val"})
	if err != nil || !ok {
		t.Fatalf("nested map match failed: ok=%v err=%v", ok, err)
	}
}

func TestMatch_OmitemptyField_Empty(t *testing.T) {
	t.Parallel()

	v := withOmitempty{Name: "x", Tag: ""}

	// omitempty field with zero value is not in the YAML output
	ok, err := filter.Match(v, []string{"tag=something"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ok {
		t.Fatal("expected no match for omitempty zero-value field")
	}
}

func TestMatch_OmitemptyField_Present(t *testing.T) {
	t.Parallel()

	v := withOmitempty{Name: "x", Tag: "important"}

	ok, err := filter.Match(v, []string{"tag=important"})
	if err != nil || !ok {
		t.Fatalf("omitempty present match failed: ok=%v err=%v", ok, err)
	}
}

func TestMatch_EmptyValue(t *testing.T) {
	t.Parallel()

	v := innerStruct{Name: ""}

	ok, err := filter.Match(v, []string{"name="})
	if err != nil || !ok {
		t.Fatalf("empty value match failed: ok=%v err=%v", ok, err)
	}
}

func TestMatch_ValueWithEquals(t *testing.T) {
	t.Parallel()

	v := innerStruct{Name: "key=value"}

	// strings.Cut splits on first =, so "name=key=value" → key="name", val="key=value"
	ok, err := filter.Match(v, []string{"name=key=value"})
	if err != nil || !ok {
		t.Fatalf("value-with-equals match failed: ok=%v err=%v", ok, err)
	}
}

func TestMatch_InvalidFilterSyntax(t *testing.T) {
	t.Parallel()

	_, err := filter.Match(innerStruct{}, []string{"noequals"})
	if err == nil {
		t.Fatal("expected error for missing =")
	}
}

func TestMatch_PlainMap(t *testing.T) {
	t.Parallel()

	// Not a struct — a raw map should also work.
	v := map[string]any{
		"name": "test",
		"nested": map[string]any{
			"key": "val",
		},
	}

	ok, err := filter.Match(v, []string{"name=test"})
	if err != nil || !ok {
		t.Fatalf("plain map match failed: ok=%v err=%v", ok, err)
	}

	ok, err = filter.Match(v, []string{"nested.key=val"})
	if err != nil || !ok {
		t.Fatalf("plain map nested match failed: ok=%v err=%v", ok, err)
	}
}

func TestMatch_ZeroStruct(t *testing.T) {
	t.Parallel()

	v := nested{}

	ok, err := filter.Match(v, []string{"inner.name="})
	if err != nil || !ok {
		t.Fatalf("zero struct empty match failed: ok=%v err=%v", ok, err)
	}

	ok, err = filter.Match(v, []string{"inner.value=0"})
	if err != nil || !ok {
		t.Fatalf("zero struct int match failed: ok=%v err=%v", ok, err)
	}
}

func TestMatch_WildcardPrefix(t *testing.T) {
	t.Parallel()

	v := innerStruct{Name: "gpt-oss:20b"}

	ok, err := filter.Match(v, []string{"name=gpt*"})
	if err != nil || !ok {
		t.Fatalf("wildcard prefix failed: ok=%v err=%v", ok, err)
	}
}

func TestMatch_WildcardSuffix(t *testing.T) {
	t.Parallel()

	v := innerStruct{Name: "claude-sonnet-4-6"}

	ok, err := filter.Match(v, []string{"name=*sonnet*"})
	if err != nil || !ok {
		t.Fatalf("wildcard contains failed: ok=%v err=%v", ok, err)
	}
}

func TestMatch_WildcardCaseInsensitive(t *testing.T) {
	t.Parallel()

	v := innerStruct{Name: "GPT-OSS:20b"}

	ok, err := filter.Match(v, []string{"name=gpt*"})
	if err != nil || !ok {
		t.Fatalf("wildcard case-insensitive failed: ok=%v err=%v", ok, err)
	}
}

func TestMatch_WildcardNoMatch(t *testing.T) {
	t.Parallel()

	v := innerStruct{Name: "claude-sonnet"}

	ok, err := filter.Match(v, []string{"name=gpt*"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ok {
		t.Fatal("expected no match for non-matching wildcard")
	}
}

func TestMatch_WildcardNested(t *testing.T) {
	t.Parallel()

	v := nested{Inner: innerStruct{Name: "https://ai.garfield-labs.com/llamacpp"}}

	ok, err := filter.Match(v, []string{"inner.name=*garfield*"})
	if err != nil || !ok {
		t.Fatalf("nested wildcard failed: ok=%v err=%v", ok, err)
	}
}

func TestMatch_ExactStillWorks(t *testing.T) {
	t.Parallel()

	v := innerStruct{Name: "exact-value"}

	ok, err := filter.Match(v, []string{"name=exact-value"})
	if err != nil || !ok {
		t.Fatalf("exact match with wildcard engine failed: ok=%v err=%v", ok, err)
	}
}
