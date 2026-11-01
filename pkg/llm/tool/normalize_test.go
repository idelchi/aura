package tool

import (
	"reflect"
	"strings"
	"testing"

	orderedmap "github.com/pb33f/ordered-map/v2"
)

func TestNormalizeDeclaredArguments(t *testing.T) {
	properties := orderedmap.New[string, Property]()
	properties.Set("changes", Property{Type: "array"})
	properties.Set("options", Property{Type: "object"})
	properties.Set("all", Property{Type: "boolean"})
	properties.Set("limit", Property{Type: "integer"})
	properties.Set("text", Property{Type: "string"})
	schema := Schema{Parameters: Parameters{Properties: properties}}
	original := map[string]any{"changes": `[{"action":"update"}]`, "options": `{"preview":true}`, "all": "True", "limit": "12", "text": "false"}
	got := schema.NormalizeArgs(original)
	if err := schema.ValidateArgs(got); err != nil {
		t.Fatal(err)
	}
	if got["all"] != true || got["limit"] != int64(12) || got["text"] != "false" {
		t.Fatal(got)
	}
	if _, ok := original["changes"].(string); !ok {
		t.Fatal("mutated the original provider call")
	}
	if note := schema.NormalizationNotes(original, got); !strings.Contains(note, "changes string -> array") || strings.Contains(note, "action") {
		t.Fatal(note)
	}
	for _, bad := range []map[string]any{
		{"all": "yes"}, {"all": "1"}, {"changes": "{}"}, {"changes": "[] trailing"},
		{"options": "null"}, {"limit": "1.5"}, {"limit": "99999999999999999999999"},
	} {
		if normalized := schema.NormalizeArgs(bad); !reflect.DeepEqual(bad, normalized) || schema.ValidateArgs(normalized) == nil {
			t.Fatalf("ambiguous or invalid value was accepted: %#v -> %#v", bad, normalized)
		}
	}
}

func TestUnparsedCallDetectionPreservesExamples(t *testing.T) {
	for _, value := range []string{"<tool_call>\n<function=Bash=\n</parameter>", "  <function=Bash>"} {
		if !HasUnparsedCall(value) {
			t.Fatal("missed malformed call", value)
		}
	}
	for _, value := range []string{"The <tool_call> tag was invalid.", "```xml\n<tool_call>\n```", "Finished."} {
		if HasUnparsedCall(value) {
			t.Fatal("treated prose as execution", value)
		}
	}
}
