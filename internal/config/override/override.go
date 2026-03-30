// Package override applies CLI --override key=value pairs to config structs.
//
// Overrides use dot-notation paths (e.g., "features.tools.max_steps=100") which
// are converted to nested YAML and loaded into the target struct via yaml.Node.Load.
// Only the specified field is touched — all other fields are preserved.
package override

import (
	"fmt"
	"strings"

	"go.yaml.in/yaml/v4"
)

// Nodes holds pre-parsed override YAML nodes for efficient re-application.
// Parsed once at startup, applied every rebuildState() turn.
type Nodes []yaml.Node

// Cache parses and validates all override strings against the target struct,
// returning reusable YAML nodes. The target is populated with the parsed
// values (useful for extracting model fields for agent.Overrides).
func Cache(target any, overrides []string) (Nodes, error) {
	nodes := make(Nodes, len(overrides))

	for i, raw := range overrides {
		path, value, err := Parse(raw)
		if err != nil {
			return nil, err
		}

		yamlStr := DotToYAML(path, value)

		if err := yaml.Unmarshal([]byte(yamlStr), &nodes[i]); err != nil {
			return nil, fmt.Errorf("override %q: %w", raw, err)
		}

		// Validate against the target struct (catches unknown fields).
		if err := nodes[i].Load(target, yaml.WithKnownFields()); err != nil {
			return nil, fmt.Errorf("override %q: %w", raw, err)
		}
	}

	return nodes, nil
}

// Apply applies cached nodes to the target struct.
// Only fields specified in the overrides are touched; all others preserved.
func (n Nodes) Apply(target any) error {
	for i := range n {
		if err := n[i].Load(target); err != nil {
			return fmt.Errorf("applying override: %w", err)
		}
	}

	return nil
}

// Apply applies a single "key.path=value" override to the target struct.
// The target must be a pointer to a struct with yaml tags.
func Apply(target any, raw string) error {
	path, value, err := Parse(raw)
	if err != nil {
		return err
	}

	yamlStr := DotToYAML(path, value)

	var node yaml.Node
	if err := yaml.Unmarshal([]byte(yamlStr), &node); err != nil {
		return fmt.Errorf("override %q: %w", raw, err)
	}

	if err := node.Load(target, yaml.WithKnownFields()); err != nil {
		return fmt.Errorf("override %q: %w", raw, err)
	}

	return nil
}

// ApplyAll applies multiple overrides in order. Last wins for the same path.
func ApplyAll(target any, overrides []string) error {
	for _, raw := range overrides {
		if err := Apply(target, raw); err != nil {
			return err
		}
	}

	return nil
}

// Parse splits "key.path=value" on the first '=' into a dot-path and value string.
func Parse(raw string) (string, string, error) {
	path, value, ok := strings.Cut(raw, "=")
	if !ok {
		return "", "", fmt.Errorf("override %q: missing '=' separator", raw)
	}

	if path == "" {
		return "", "", fmt.Errorf("override %q: empty key", raw)
	}

	return path, value, nil
}

// DotToYAML converts a dot-path and value into a nested YAML document.
//
//	"tools.bash.truncation.max_lines" + "300" →
//	tools:
//	  bash:
//	    truncation:
//	      max_lines: 300
func DotToYAML(path, value string) string {
	parts := strings.Split(path, ".")

	var sb strings.Builder

	for i, part := range parts {
		for range i {
			sb.WriteString("  ")
		}

		if i == len(parts)-1 {
			if needsQuoting(value) {
				fmt.Fprintf(&sb, "%s: %q\n", part, value)
			} else {
				fmt.Fprintf(&sb, "%s: %s\n", part, value)
			}
		} else {
			fmt.Fprintf(&sb, "%s:\n", part)
		}
	}

	return sb.String()
}

// needsQuoting returns true if the value contains characters or words that
// YAML would misinterpret.
func needsQuoting(s string) bool {
	if s == "" {
		return true // empty must be quoted to avoid YAML null
	}

	// Flow sequences/mappings — let YAML parse them as typed values
	if s[0] == '[' || s[0] == '{' {
		return false
	}

	// YAML reserved words that would be parsed as null (and silently ignored)
	lower := strings.ToLower(s)
	if lower == "null" || lower == "~" {
		return true
	}

	// Characters that YAML gives special meaning to
	return strings.ContainsAny(s, ":{}[]|>!&*#?%@`")
}
