package config

import (
	"reflect"
	"testing"
)

// TestTemplateContextFields keeps the extensible context aligned with every shared field.
func TestTemplateContextFields(t *testing.T) {
	data := TemplateData{Model: ModelData{Name: "sample"}, Provider: ProviderData{Name: "ollama", URL: "http://localhost:11434"}}
	value := reflect.ValueOf(data)
	context := data.Context()
	if len(context) != value.NumField() {
		t.Fatal("shared template context has missing or extra fields")
	}
	for i := range value.NumField() {
		name := value.Type().Field(i).Name
		if !reflect.DeepEqual(context[name], value.Field(i).Interface()) {
			t.Errorf("context field %s differs from prompt data", name)
		}
	}
}
