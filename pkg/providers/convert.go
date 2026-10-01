package providers

import (
	"github.com/idelchi/aura/pkg/llm/tool"
	orderedmap "github.com/pb33f/ordered-map/v2"
)

// Ptr returns a pointer to the given value.
//
//go:fix inline
func Ptr[T any](v T) *T { return new(v) }

// BuildPropertyMap retains declaration order for JSON-based SDKs. Pass false
// only for SDKs that inspect ordinary maps instead of marshaling their values.
func BuildPropertyMap(props *tool.Properties, preserveOrder bool) any {
	if preserveOrder {
		properties := orderedmap.New[string, any]()
		for name, prop := range props.FromOldest() {
			properties.Set(name, BuildPropertyEntry(prop, true))
		}
		return properties
	}

	// Some SDKs inspect ordinary maps instead of marshaling arbitrary JSON values.
	properties := make(map[string]any, props.Len())
	for name, prop := range props.FromOldest() {
		properties[name] = BuildPropertyEntry(prop, false)
	}
	return properties
}

// BuildPropertyEntry converts a single Property to a JSON Schema map, recursing into nested types.
func BuildPropertyEntry(prop tool.Property, preserveOrder bool) map[string]any {
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
		m["items"] = BuildPropertyEntry(*prop.Items, preserveOrder)
	}

	if prop.Properties.Len() > 0 {
		m["properties"] = BuildPropertyMap(prop.Properties, preserveOrder)
	}

	if len(prop.Required) > 0 {
		m["required"] = prop.Required
	}

	return m
}

// BuildParametersMap converts tool parameters to the full JSON Schema map
// format used by OpenAI and OpenRouter.
func BuildParametersMap(p tool.Parameters, preserveOrder bool) map[string]any {
	result := map[string]any{
		"type":       p.Type,
		"properties": BuildPropertyMap(p.Properties, preserveOrder),
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
