// Package filter provides generic key=value filtering for config structs
// using YAML round-trip. Structs are marshaled to YAML, unmarshaled to
// map[string]any, then queried with dot-path keys (e.g., "model.name").
package filter

import (
	"fmt"
	"strings"

	"github.com/idelchi/aura/pkg/wildcard"

	"go.yaml.in/yaml/v4"
)

// Match returns true if the struct matches all key=value filters.
// Keys use dot notation for nested fields (e.g., "model.name").
// Values support wildcard patterns (e.g., "gpt*") and are matched case-insensitively.
func Match(v any, filters []string) (bool, error) {
	if len(filters) == 0 {
		return true, nil
	}

	data, err := yaml.Marshal(v)
	if err != nil {
		return false, fmt.Errorf("marshaling for filter: %w", err)
	}

	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		return false, fmt.Errorf("unmarshaling for filter: %w", err)
	}

	for _, f := range filters {
		key, val, ok := strings.Cut(f, "=")
		if !ok {
			return false, fmt.Errorf("invalid filter %q: expected key=value", f)
		}

		actual := strings.ToLower(fmt.Sprint(resolve(m, key)))
		if !wildcard.Match(actual, strings.ToLower(val)) {
			return false, nil
		}
	}

	return true, nil
}

// resolve walks a dot-separated key path through a nested map.
func resolve(m map[string]any, key string) any {
	parts := strings.Split(key, ".")

	var current any = m

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]any:
			current = v[part]
		default:
			return nil
		}
	}

	return current
}
