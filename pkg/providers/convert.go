package providers

import "github.com/idelchi/aura/pkg/llm/tool"

// Ptr returns a pointer to the given value.
//
//go:fix inline
func Ptr[T any](v T) *T { return new(v) }

// BuildPropertyMap converts tool properties to the generic map format
// used by OpenAI-compatible provider SDKs.
func BuildPropertyMap(props map[string]tool.Property) map[string]any {
	properties := make(map[string]any, len(props))

	for name, prop := range props {
		properties[name] = BuildPropertyEntry(prop)
	}

	return properties
}

// BuildPropertyEntry converts a single Property to a JSON Schema map, recursing into nested types.
func BuildPropertyEntry(prop tool.Property) map[string]any {
	m := map[string]any{
		"type": prop.Type,
	}

	if prop.Description != "" {
		m["description"] = prop.Description
	}

	if len(prop.Enum) > 0 {
		m["enum"] = prop.Enum
	}

	if prop.Items != nil {
		m["items"] = BuildPropertyEntry(*prop.Items)
	}

	if len(prop.Properties) > 0 {
		m["properties"] = BuildPropertyMap(prop.Properties)
	}

	if len(prop.Required) > 0 {
		m["required"] = prop.Required
	}

	return m
}

// BuildParametersMap converts tool parameters to the full JSON Schema map
// format used by OpenAI and OpenRouter.
func BuildParametersMap(p tool.Parameters) map[string]any {
	result := map[string]any{
		"type":       p.Type,
		"properties": BuildPropertyMap(p.Properties),
	}

	if len(p.Required) > 0 {
		result["required"] = p.Required
	}

	return result
}

// Float32sToFloat64s converts a slice of float32 values to float64.
func Float32sToFloat64s(vals []float32) []float64 {
	result := make([]float64, len(vals))
	for i, v := range vals {
		result[i] = float64(v)
	}

	return result
}
