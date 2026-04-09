package inherit_test

import (
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/config/inherit"
)

type testItem struct {
	Name    string
	Value   int
	Parents []string
}

func parents(item testItem) []string { return item.Parents }

func TestResolveNoParents(t *testing.T) {
	t.Parallel()

	items := map[string]testItem{
		"a": {Name: "alpha", Value: 1},
		"b": {Name: "beta", Value: 2},
	}

	got, err := inherit.Resolve(items, parents)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}

	if got["a"].Value != 1 {
		t.Errorf("a.Value = %d, want 1", got["a"].Value)
	}

	if got["b"].Value != 2 {
		t.Errorf("b.Value = %d, want 2", got["b"].Value)
	}
}

func TestResolveLinearChain(t *testing.T) {
	t.Parallel()

	items := map[string]testItem{
		"a": {Name: "alpha", Value: 1, Parents: []string{"b"}},
		"b": {Name: "beta", Value: 2, Parents: []string{"c"}},
		"c": {Name: "gamma", Value: 3},
	}

	got, err := inherit.Resolve(items, parents)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// Child overrides: a.Value=1 wins over inherited
	if got["a"].Value != 1 {
		t.Errorf("a.Value = %d, want 1 (child override)", got["a"].Value)
	}

	if got["b"].Value != 2 {
		t.Errorf("b.Value = %d, want 2", got["b"].Value)
	}

	if got["c"].Value != 3 {
		t.Errorf("c.Value = %d, want 3", got["c"].Value)
	}
}

func TestResolveParentFieldInherited(t *testing.T) {
	t.Parallel()

	items := map[string]testItem{
		"child":  {Name: "child", Value: 0, Parents: []string{"parent"}},
		"parent": {Name: "parent", Value: 5},
	}

	got, err := inherit.Resolve(items, parents)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// Zero value on child → mergo fills from parent
	if got["child"].Value != 5 {
		t.Errorf("child.Value = %d, want 5 (inherited from parent)", got["child"].Value)
	}

	// Name is non-zero on child, should be preserved
	if got["child"].Name != "child" {
		t.Errorf("child.Name = %q, want %q", got["child"].Name, "child")
	}
}

func TestResolveDAG(t *testing.T) {
	t.Parallel()

	// Diamond: D inherits B+C, both inherit A
	items := map[string]testItem{
		"a": {Name: "base", Value: 1},
		"b": {Name: "left", Value: 2, Parents: []string{"a"}},
		"c": {Name: "right", Value: 3, Parents: []string{"a"}},
		"d": {Name: "", Value: 0, Parents: []string{"b", "c"}},
	}

	got, err := inherit.Resolve(items, parents)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// D has zero Name → should inherit. Parents [b, c] merged left-to-right with override,
	// so c's Name="right" wins over b's Name="left", then d overlays (but d.Name is zero, no override).
	if got["d"].Name != "right" {
		t.Errorf("d.Name = %q, want %q (last parent wins)", got["d"].Name, "right")
	}

	// D.Value is 0 → should inherit from last parent c.Value=3
	if got["d"].Value != 3 {
		t.Errorf("d.Value = %d, want 3 (inherited from last parent)", got["d"].Value)
	}
}

func TestResolveCycleError(t *testing.T) {
	t.Parallel()

	items := map[string]testItem{
		"a": {Parents: []string{"b"}},
		"b": {Parents: []string{"a"}},
	}

	_, err := inherit.Resolve(items, parents)
	if err == nil {
		t.Fatal("expected error for cycle, got nil")
	}

	if !strings.Contains(strings.ToLower(err.Error()), "cycle") {
		t.Errorf("error = %q, want it to contain 'cycle'", err.Error())
	}
}

func TestResolveMissingParentError(t *testing.T) {
	t.Parallel()

	items := map[string]testItem{
		"a": {Parents: []string{"nonexistent"}},
	}

	_, err := inherit.Resolve(items, parents)
	if err == nil {
		t.Fatal("expected error for missing parent, got nil")
	}
}

func TestResolveOverrideOrder(t *testing.T) {
	t.Parallel()

	items := map[string]testItem{
		"base1": {Name: "first", Value: 10},
		"base2": {Name: "second", Value: 20},
		"child": {Name: "", Value: 0, Parents: []string{"base1", "base2"}},
	}

	got, err := inherit.Resolve(items, parents)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// Parents merged left-to-right with WithOverride: base2 overrides base1
	if got["child"].Value != 20 {
		t.Errorf("child.Value = %d, want 20 (last parent wins)", got["child"].Value)
	}

	if got["child"].Name != "second" {
		t.Errorf("child.Name = %q, want %q (last parent wins)", got["child"].Name, "second")
	}
}
