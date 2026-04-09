package tool

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/invopop/jsonschema"
)

// Schemas is a collection of tool schemas.
type Schemas []Schema

// Schema represents a provider-agnostic tool definition.
type Schema struct {
	Name        string
	Description string
	Parameters  Parameters
}

// ParamNames returns sorted parameter names for error messages.
func (s Schema) ParamNames() []string {
	return slices.Sorted(maps.Keys(s.Parameters.Properties))
}

// ValidateArgs validates dynamic arguments against the schema.
// It collects all errors before returning, so the caller can fix everything in one pass.
func (s Schema) ValidateArgs(args map[string]any) error {
	var errs []string

	// Unknown fields.
	for key := range args {
		if _, ok := s.Parameters.Properties[key]; !ok {
			errs = append(errs, fmt.Sprintf("unknown parameter %q", key))
		}
	}

	// Required params.
	for _, name := range s.Parameters.Required {
		if _, ok := args[name]; !ok {
			errs = append(errs, fmt.Sprintf("missing required parameter %q", name))
		}
	}

	// Type + enum checks for present params.
	for name, prop := range s.Parameters.Properties {
		val, ok := args[name]
		if !ok {
			continue
		}

		if prop.Type != "" {
			if msg := checkType(name, val, prop.Type); msg != "" {
				errs = append(errs, msg)
			}
		}

		if len(prop.Enum) > 0 && !enumContains(prop.Enum, val) {
			allowed := make([]string, len(prop.Enum))
			for i, e := range prop.Enum {
				allowed[i] = fmt.Sprint(e)
			}

			errs = append(errs, fmt.Sprintf("parameter %q: value %q is not one of: %s",
				name, fmt.Sprint(val), strings.Join(allowed, ", ")))
		}
	}

	if len(errs) == 0 {
		return nil
	}

	return errors.New(strings.Join(errs, "\n") + "\n\nValid parameters: " + strings.Join(s.ParamNames(), ", "))
}

// checkType validates that val matches the declared JSON schema type.
// Returns an error message string, or empty if valid.
func checkType(name string, val any, typ string) string {
	ok := false

	switch typ {
	case "string":
		_, ok = val.(string)
	case "integer":
		switch v := val.(type) {
		case float64:
			ok = math.Trunc(v) == v
		case int, int64:
			ok = true
		}
	case "number":
		switch val.(type) {
		case float64, int, int64:
			ok = true
		}
	case "boolean":
		_, ok = val.(bool)
	case "array":
		_, ok = val.([]any)
	case "object":
		_, ok = val.(map[string]any)
	default:
		return "" // unknown type — skip check
	}

	if !ok {
		return fmt.Sprintf("parameter %q: expected %s, got %T", name, typ, val)
	}

	return ""
}

// enumContains checks whether val is in the allowed enum values.
func enumContains(enum []any, val any) bool {
	s := fmt.Sprint(val)

	for _, e := range enum {
		if fmt.Sprint(e) == s {
			return true
		}
	}

	return false
}

// Parameters describes the input parameters for a tool.
type Parameters struct {
	Type       string
	Properties map[string]Property
	Required   []string
}

// Property describes a single parameter.
// Supports nested schemas: Items for array element types, Properties/Required for object fields.
type Property struct {
	Type        string
	Description string
	Enum        []any

	// Items describes the element type for arrays (e.g., array of objects).
	Items *Property

	// Properties and Required describe fields for object types.
	Properties map[string]Property
	Required   []string
}

// ComposeDescription builds the full tool description from Description, Usage, and Examples.
// Used by GenerateSchema and by tools that build Schema dynamically (plugins).
func ComposeDescription(t Tool) string {
	return heredoc.Docf(`
		%s

		Usage:
		%s

		Examples:
		%s
	`, t.Description(), t.Usage(), t.Examples())
}

// GenerateSchema creates a tool schema from a type using JSON schema reflection.
func GenerateSchema[T any](t Tool) Schema {
	description := ComposeDescription(t)

	reflector := jsonschema.Reflector{
		DoNotReference: true,
	}

	reflected := reflector.Reflect(new(T))

	properties := make(map[string]Property)

	if reflected.Properties != nil {
		for pair := reflected.Properties.Oldest(); pair != nil; pair = pair.Next() {
			properties[pair.Key] = convertProperty(pair.Value)
		}
	}

	required := []string{}

	if reflected.Required != nil {
		required = reflected.Required
	}

	return Schema{
		Name:        t.Name(),
		Description: description,
		Parameters: Parameters{
			Type:       "object",
			Properties: properties,
			Required:   required,
		},
	}
}

func convertProperty(schema *jsonschema.Schema) Property {
	prop := Property{
		Description: schema.Description,
	}

	if schema.Type != "" {
		prop.Type = schema.Type
	}

	if len(schema.Enum) > 0 {
		prop.Enum = schema.Enum
	}

	// Recurse into array items.
	if schema.Items != nil {
		items := convertProperty(schema.Items)

		prop.Items = &items
	}

	// Recurse into object properties.
	if schema.Properties != nil {
		prop.Properties = make(map[string]Property)

		for pair := schema.Properties.Oldest(); pair != nil; pair = pair.Next() {
			prop.Properties[pair.Key] = convertProperty(pair.Value)
		}
	}

	if len(schema.Required) > 0 {
		prop.Required = schema.Required
	}

	return prop
}

// Render returns a text representation of the schema for token estimation.
func (s Schema) Render() string {
	var b strings.Builder

	fmt.Fprintf(&b, "tool: %s\n%s\n", s.Name, s.Description)

	for name, prop := range s.Parameters.Properties {
		fmt.Fprintf(&b, "  param %s (%s): %s\n", name, prop.Type, prop.Description)

		if len(prop.Enum) > 0 {
			for _, e := range prop.Enum {
				fmt.Fprintf(&b, "    - %v\n", e)
			}
		}
	}

	return b.String()
}

// Render returns a text representation of all schemas for token estimation.
func (ss Schemas) Render() string {
	var b strings.Builder

	for _, s := range ss {
		b.WriteString(s.Render())
	}

	return b.String()
}
