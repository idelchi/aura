package tool

import (
	"encoding/json"
	"fmt"
	"maps"
	"strconv"
	"strings"
)

// NormalizeArgs converts only explicit string representations of a declared
// JSON container, boolean or integer. String parameters and ambiguous values
// remain untouched, so ordinary validation still reports them.
func (s Schema) NormalizeArgs(args map[string]any) map[string]any {
	result := maps.Clone(args)
	for name, prop := range s.Parameters.Properties.FromOldest() {
		if value, exists := result[name]; exists {
			result[name] = prop.normalize(value)
		}
	}
	return result
}

func (p Property) normalize(value any) any {
	if text, ok := value.(string); ok {
		switch p.Type {
		case "boolean":
			switch strings.ToLower(strings.TrimSpace(text)) {
			case "true":
				value = true
			case "false":
				value = false
			}
		case "integer":
			if number, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64); err == nil {
				value = number
			}
		case "array", "object":
			var decoded any
			if json.Unmarshal([]byte(text), &decoded) == nil && checkType("", decoded, p.Type) == "" {
				value = decoded
			}
		}
	}
	if object, ok := value.(map[string]any); ok && p.Properties != nil {
		copy := maps.Clone(object)
		for name, child := range p.Properties.FromOldest() {
			if current, exists := copy[name]; exists {
				copy[name] = child.normalize(current)
			}
		}
		return copy
	}
	if array, ok := value.([]any); ok && p.Items != nil {
		copy := make([]any, len(array))
		for i, current := range array {
			copy[i] = p.Items.normalize(current)
		}
		return copy
	}
	return value
}

// NormalizationNotes explains conversions without copying argument values into
// diagnostics. Config-bound fields are absent from the advertised schema.
func (s Schema) NormalizationNotes(before, after map[string]any) string {
	var notes []string
	for name, prop := range s.Parameters.Properties.FromOldest() {
		notes = append(notes, prop.normalizationNotes(name, before[name], after[name])...)
	}
	if len(notes) == 0 {
		return ""
	}
	return "Normalized tool arguments: " + strings.Join(notes, "; ") + ". Use native JSON values next time."
}

func (p Property) normalizationNotes(path string, before, after any) []string {
	if _, ok := before.(string); ok && p.Type != "" && p.Type != "string" && checkType(path, after, p.Type) == "" {
		return []string{path + " string -> " + p.Type}
	}
	var notes []string
	left, leftOK := before.(map[string]any)
	right, rightOK := after.(map[string]any)
	if leftOK && rightOK && p.Properties != nil {
		for name, prop := range p.Properties.FromOldest() {
			notes = append(notes, prop.normalizationNotes(path+"."+name, left[name], right[name])...)
		}
	}
	a, aOK := before.([]any)
	b, bOK := after.([]any)
	if aOK && bOK && len(a) == len(b) && p.Items != nil {
		for i := range a {
			notes = append(notes, p.Items.normalizationNotes(fmt.Sprintf("%s[%d]", path, i), a[i], b[i])...)
		}
	}
	return notes
}
