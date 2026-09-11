// Package responseformat defines structured output constraints for LLM responses.
package responseformat

import (
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
)

// Type enumerates the allowed response format types.
type Type string

const (
	Text       Type = "text"
	JSONObject Type = "json_object"
	JSONSchema Type = "json_schema"
)

// ResponseFormat constrains the LLM's response to a specific format.
type ResponseFormat struct {
	// Type is one of "text", "json_object", "json_schema".
	Type Type `yaml:"type"`
	// Name identifies the schema (required by OpenAI for json_schema).
	Name string `yaml:"name"`
	// Schema is the JSON Schema as a Go map (required for json_schema).
	Schema map[string]any `yaml:"schema"`
	// Strict enables strict schema adherence when supported (OpenAI, OpenRouter).
	Strict bool `yaml:"strict"`
}

// EffectiveName returns the schema name, defaulting to "response" when empty.
// OpenAI requires a non-empty name for json_schema; others ignore it.
func (r *ResponseFormat) EffectiveName() string {
	if r.Name == "" {
		return "response"
	}

	return r.Name
}

// Validate checks the declared output contract without judging its factual content.
func (r *ResponseFormat) Validate(content string) error {
	if r == nil || r.Type == Text || r.Type == "" {
		return nil
	}
	var value any
	if err := json.Unmarshal([]byte(content), &value); err != nil {
		return err
	}
	if r.Type == JSONObject {
		if _, ok := value.(map[string]any); !ok {
			return fmt.Errorf("response must be a JSON object")
		}
		return nil
	}
	data, err := json.Marshal(r.Schema)
	if err != nil {
		return err
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(data, &schema); err != nil {
		return err
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		return err
	}
	return resolved.Validate(value)
}
