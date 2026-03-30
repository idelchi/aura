package ollama

import (
	"github.com/ollama/ollama/api"

	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/aura/pkg/providers"
)

// ToTools converts tool schemas to Ollama format.
func ToTools(schemas []tool.Schema) []api.Tool {
	tools := make([]api.Tool, len(schemas))
	for i, s := range schemas {
		tools[i] = ToTool(s)
	}

	return tools
}

// ToTool converts a tool schema to Ollama format.
func ToTool(s tool.Schema) api.Tool {
	properties := convertProperties(s.Parameters.Properties)

	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        s.Name,
			Description: s.Description,
			Parameters: api.ToolFunctionParameters{
				Type:       s.Parameters.Type,
				Properties: properties,
				Required:   s.Parameters.Required,
			},
		},
	}
}

// convertProperties recursively converts tool properties to Ollama format.
func convertProperties(props map[string]tool.Property) *api.ToolPropertiesMap {
	properties := api.NewToolPropertiesMap()

	for name, prop := range props {
		properties.Set(name, convertProperty(prop))
	}

	return properties
}

// convertProperty converts a single tool property, recursing into nested types.
func convertProperty(prop tool.Property) api.ToolProperty {
	tp := api.ToolProperty{
		Type:        api.PropertyType{prop.Type},
		Description: prop.Description,
		Enum:        prop.Enum,
	}

	if prop.Items != nil {
		// Items is typed as `any` in the Ollama SDK, so we can pass a rich map
		// that includes `required` (which ToolProperty doesn't support directly).
		tp.Items = providers.BuildPropertyEntry(*prop.Items)
	}

	if len(prop.Properties) > 0 {
		tp.Properties = convertProperties(prop.Properties)
	}

	return tp
}
