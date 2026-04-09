package plugins

import (
	"testing"
)

func TestMatchesFiltersNoFilters(t *testing.T) {
	t.Parallel()

	if !matchesFilters("foo", nil, nil) {
		t.Errorf("matchesFilters(%q, nil, nil) = false; want true", "foo")
	}
}

func TestMatchesFiltersIncludeMatch(t *testing.T) {
	t.Parallel()

	if !matchesFilters("foo", []string{"foo", "bar"}, nil) {
		t.Errorf("matchesFilters(%q, [foo bar], nil) = false; want true", "foo")
	}
}

func TestMatchesFiltersIncludeNoMatch(t *testing.T) {
	t.Parallel()

	if matchesFilters("baz", []string{"foo"}, nil) {
		t.Errorf("matchesFilters(%q, [foo], nil) = true; want false", "baz")
	}
}

func TestMatchesFiltersExclude(t *testing.T) {
	t.Parallel()

	if matchesFilters("foo", nil, []string{"foo"}) {
		t.Errorf("matchesFilters(%q, nil, [foo]) = true; want false", "foo")
	}
}

func TestMatchesFiltersExcludeNoMatch(t *testing.T) {
	t.Parallel()

	if !matchesFilters("bar", nil, []string{"foo"}) {
		t.Errorf("matchesFilters(%q, nil, [foo]) = false; want true", "bar")
	}
}

func TestMatchesFiltersIncludeAndExclude(t *testing.T) {
	t.Parallel()

	if matchesFilters("bar", []string{"foo", "bar"}, []string{"bar"}) {
		t.Errorf("matchesFilters(%q, [foo bar], [bar]) = true; want false", "bar")
	}
}

func TestMatchesFiltersWildcardInclude(t *testing.T) {
	t.Parallel()

	if !matchesFilters("my-plugin", []string{"my-*"}, nil) {
		t.Errorf("matchesFilters(%q, [my-*], nil) = false; want true", "my-plugin")
	}
}

func TestMatchesFiltersWildcardExclude(t *testing.T) {
	t.Parallel()

	if matchesFilters("my-plugin", nil, []string{"my-*"}) {
		t.Errorf("matchesFilters(%q, nil, [my-*]) = true; want false", "my-plugin")
	}
}

func TestMatchesFiltersWildcardNoMatch(t *testing.T) {
	t.Parallel()

	if matchesFilters("other-plugin", []string{"my-*"}, nil) {
		t.Errorf("matchesFilters(%q, [my-*], nil) = true; want false", "other-plugin")
	}
}
