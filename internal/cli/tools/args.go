package tools

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/idelchi/aura/pkg/llm/tool"
)

// parseArgs determines the input format and returns parsed arguments.
// If rawJSON is true or the input looks like JSON, it parses as JSON.
// Otherwise, it parses as key=value pairs with schema-driven type coercion.
func parseArgs(remaining []string, schema tool.Schema, rawJSON bool) (map[string]any, error) {
	// Single arg starting with '{' or --raw flag: parse as JSON.
	if rawJSON || (len(remaining) == 1 && strings.HasPrefix(strings.TrimSpace(remaining[0]), "{")) {
		var argsMap map[string]any
		if err := json.Unmarshal([]byte(strings.Join(remaining, " ")), &argsMap); err != nil {
			return nil, fmt.Errorf("invalid JSON args: %w", err)
		}

		return argsMap, nil
	}

	return parseKVArgs(remaining, schema)
}

// parseKVArgs parses key=value pairs and coerces values based on the schema.
//
// TODO(idelchi): Several tools have nested params (array/object) that cannot be expressed
// as key=value pairs. Consider dotted-path syntax (items.0.content=foo) for flat→nested
// conversion. For now, those tools require JSON input (--raw or '{...}').
func parseKVArgs(pairs []string, schema tool.Schema) (map[string]any, error) {
	result := make(map[string]any, len(pairs))

	for _, pair := range pairs {
		key, value, ok := strings.Cut(pair, "=")
		if !ok {
			return nil, fmt.Errorf("invalid argument %q — expected key=value format", pair)
		}

		prop, exists := schema.Parameters.Properties[key]
		if !exists {
			return nil, fmt.Errorf("unknown parameter %q\nValid parameters: %s",
				key, strings.Join(schema.ParamNames(), ", "))
		}

		coerced, err := coerceValue(key, value, prop.Type)
		if err != nil {
			return nil, err
		}

		result[key] = coerced
	}

	return result, nil
}

// coerceValue converts a string value to the type declared in the schema.
func coerceValue(key, value, typ string) (any, error) {
	switch typ {
	case "string", "":
		return value, nil

	case "integer":
		n, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("parameter %q: expected integer, got %q", key, value)
		}

		return n, nil

	case "number":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, fmt.Errorf("parameter %q: expected number, got %q", key, value)
		}

		return f, nil

	case "boolean":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return nil, fmt.Errorf("parameter %q: expected boolean, got %q", key, value)
		}

		return b, nil

	case "array", "object":
		return nil, fmt.Errorf("parameter %q has type %s — use JSON input for nested structures", key, typ)

	default:
		return value, nil
	}
}
