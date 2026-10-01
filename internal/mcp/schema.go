package mcp

import (
	"maps"
	"slices"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/pkg/llm/tool"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	orderedmap "github.com/pb33f/ordered-map/v2"
)

// convertSchema converts an MCP tool to tool.Schema.
func convertSchema(name string, mcpTool mcplib.Tool) tool.Schema {
	params := tool.Parameters{
		Type:       "object",
		Properties: orderedmap.New[string, tool.Property](),
	}

	if len(mcpTool.InputSchema.Required) > 0 {
		params.Required = mcpTool.InputSchema.Required
	}

	// The MCP SDK exposes unordered maps; stabilize their order at this boundary.
	for _, propName := range slices.Sorted(maps.Keys(mcpTool.InputSchema.Properties)) {
		propVal := mcpTool.InputSchema.Properties[propName]
		if propMap, ok := propVal.(map[string]any); ok {
			params.Properties.Set(propName, convertProperty(name, propName, propMap))
		} else {
			debug.Log("[mcp] tool %s: property %q has unexpected type %T, skipping", name, propName, propVal)
		}
	}

	return tool.Schema{
		Name:        name,
		Description: mcpTool.Description,
		Parameters:  params,
	}
}

// convertProperty extracts a tool.Property from a JSON Schema property map.
// Handles nested schemas: Items for arrays, Properties/Required for objects.
func convertProperty(toolName, propName string, propMap map[string]any) tool.Property {
	prop := tool.Property{}

	if t, ok := propMap["type"].(string); ok {
		prop.Type = t
	}

	if d, ok := propMap["description"].(string); ok {
		prop.Description = d
	}

	if e, ok := propMap["enum"].([]any); ok {
		prop.Enum = e
	}

	// Nested: array items
	if items, ok := propMap["items"].(map[string]any); ok {
		itemProp := convertProperty(toolName, propName+".items", items)

		prop.Items = &itemProp
	}

	// Nested: object properties
	if props, ok := propMap["properties"].(map[string]any); ok {
		prop.Properties = orderedmap.New[string, tool.Property]()

		for _, childName := range slices.Sorted(maps.Keys(props)) {
			childVal := props[childName]
			if childMap, ok := childVal.(map[string]any); ok {
				prop.Properties.Set(childName, convertProperty(toolName, propName+"."+childName, childMap))
			} else {
				debug.Log(
					"[mcp] tool %s: property %s.%s has unexpected type %T, skipping",
					toolName,
					propName,
					childName,
					childVal,
				)
			}
		}
	}

	// Nested: object required fields
	if req, ok := propMap["required"].([]any); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				prop.Required = append(prop.Required, s)
			}
		}
	}

	return prop
}
