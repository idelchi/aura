package responseformat

import "testing"

func TestValidateResponseContract(t *testing.T) {
	schema := &ResponseFormat{Type: JSONSchema, Schema: map[string]any{
		"type": "object", "required": []string{"answer"}, "additionalProperties": false,
		"properties": map[string]any{"answer": map[string]any{"type": "string"}},
	}}
	for _, content := range []string{`{}`, `{"answer":42}`, `{"answer":"ok","extra":true}`, "thinking only"} {
		if err := schema.Validate(content); err == nil {
			t.Errorf("accepted %s", content)
		}
	}
	if err := schema.Validate(`{"answer":"ok"}`); err != nil {
		t.Fatal(err)
	}
	if err := (&ResponseFormat{Type: JSONObject}).Validate(`[]`); err == nil {
		t.Fatal("accepted array as object")
	}
	if err := (*ResponseFormat)(nil).Validate(""); err != nil {
		t.Fatal(err)
	}
}
