package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
)

// Bindings fixes top-level tool arguments by exact tool name. Bound arguments
// are hidden from the model schema and replace caller-supplied values.
type Bindings map[string]map[string]any

// Bind returns a new tool collection; neither tools nor their schemas are mutated.
// Validation concerns available tools; bindings for excluded tools remain inactive.
func (b Bindings) Bind(tools Tools) (Tools, error) {
	bound := slices.Clone(tools)
	for i, t := range bound {
		original := t
		if previous, ok := t.(*Bound); ok {
			original = previous.Tool
		}
		bound[i] = original
		values := b[t.Name()]
		if len(values) == 0 {
			continue
		}
		schema := original.Schema()
		schema.Parameters.Required = nil
		if err := schema.ValidateArgs(values); err != nil {
			return nil, fmt.Errorf("bindings for %s: %w", t.Name(), err)
		}
		fixed, err := cloneArguments(values)
		if err != nil {
			return nil, fmt.Errorf("bindings for %s: %w", t.Name(), err)
		}
		bound[i] = &Bound{Tool: original, fixed: fixed}
	}
	return bound, nil
}

// Expand applies the caller's existing template renderer to string values while
// preserving JSON types and leaving the reusable configuration untouched.
func (b Bindings) Expand(expand func(string) (string, error)) (Bindings, error) {
	if b == nil {
		return nil, nil
	}
	result := make(Bindings, len(b))
	for name, args := range b {
		value, err := expandBinding(args, expand)
		if err != nil {
			return nil, fmt.Errorf("bindings for %s: %w", name, err)
		}
		result[name] = value.(map[string]any)
	}
	return result, nil
}

// expandBinding recursively renders string leaves in structured fixed arguments.
func expandBinding(value any, expand func(string) (string, error)) (any, error) {
	switch v := value.(type) {
	case string:
		return expand(v)
	case map[string]any:
		result := make(map[string]any, len(v))
		for key, item := range v {
			resolved, err := expandBinding(item, expand)
			if err != nil {
				return nil, err
			}
			result[key] = resolved
		}
		return result, nil
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			resolved, err := expandBinding(item, expand)
			if err != nil {
				return nil, err
			}
			result[i] = resolved
		}
		return result, nil
	default:
		return value, nil
	}
}

// Bound presents a reduced schema. Execution pipelines call Prepare before their
// policy, validation and lifecycle hooks to recover the original tool interfaces.
type Bound struct {
	Tool                 // original implementation, including its text and availability
	fixed map[string]any // private immutable JSON argument values
}

// Schema hides fixed fields and removes their model-facing required entries.
func (b *Bound) Schema() Schema {
	schema := b.Tool.Schema()
	schema.Parameters.Properties = maps.Clone(schema.Parameters.Properties)
	schema.Parameters.Required = slices.Clone(schema.Parameters.Required)
	for name := range b.fixed {
		delete(schema.Parameters.Properties, name)
		schema.Parameters.Required = slices.DeleteFunc(schema.Parameters.Required, func(key string) bool { return key == name })
	}
	return schema
}

// Execute retains bindings for callers that do not have a separate preflight.
func (b *Bound) Execute(ctx context.Context, args map[string]any) (string, error) {
	t, prepared, err := Prepare(b, args)
	if err != nil {
		return "", err
	}
	return t.Execute(ctx, prepared)
}

// Prepare returns the original tool and fully bound arguments. Call before
// hooks/path/approval checks, and again after a plugin rewrites arguments.
// Original implementations retain their optional interfaces, including sandboxing.
func Prepare(t Tool, args map[string]any) (Tool, map[string]any, error) {
	b, ok := t.(*Bound)
	if !ok {
		return t, args, nil
	}
	prepared := maps.Clone(args)
	if prepared == nil {
		prepared = make(map[string]any)
	}
	fixed, err := cloneArguments(b.fixed)
	if err != nil {
		return nil, nil, err
	}
	maps.Copy(prepared, fixed)
	if err := b.Tool.Schema().ValidateArgs(prepared); err != nil {
		return nil, nil, err
	}
	return b.Tool, prepared, nil
}

// cloneArguments isolates nested JSON values from mutation by tools and plugins.
func cloneArguments(args map[string]any) (map[string]any, error) {
	data, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	var cloned map[string]any
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil, err
	}
	return cloned, nil
}
