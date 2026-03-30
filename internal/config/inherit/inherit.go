// Package inherit provides struct-level config inheritance with DAG resolution.
//
// Config entities (agents, modes, tasks) can declare `inherit: [Parent1, Parent2]`
// to extend other definitions. Merge semantics: nil pointer = inherit from parent,
// non-nil = override. Slices: nil = inherit, non-nil (including empty) = override —
// so `disabled: []` in a child clears the parent's list.
package inherit

import (
	"fmt"

	"github.com/idelchi/aura/internal/config/merge"
	"github.com/idelchi/godyl/pkg/dag"
)

// Resolve takes named items, builds a DAG from their Inherit fields,
// and merges structs in topological order using mergo.
//
// parentsFn extracts the parent names from an item.
// Hard errors on cycles and missing parents.
func Resolve[T any](items map[string]T, parentsFn func(T) []string) (map[string]T, error) {
	names := make([]string, 0, len(items))
	for name := range items {
		names = append(names, name)
	}

	g, err := dag.Build(names, func(n string) []string {
		return parentsFn(items[n])
	})
	if err != nil {
		return nil, fmt.Errorf("config inheritance: %w", err)
	}

	resolved := make(map[string]T, len(items))

	for _, name := range g.Topo() {
		item := items[name]
		parents := parentsFn(item)

		if len(parents) == 0 {
			resolved[name] = item

			continue
		}

		// Merge parents left-to-right, then overlay child last.
		var merged T

		for _, p := range parents {
			if err := merge.Merge(&merged, resolved[p]); err != nil {
				return nil, fmt.Errorf("merging %q from parent %q: %w", name, p, err)
			}
		}

		if err := merge.Merge(&merged, item); err != nil {
			return nil, fmt.Errorf("merging %q (child overlay): %w", name, err)
		}

		resolved[name] = merged
	}

	return resolved, nil
}
