package tmpl

import (
	"bytes"
	"fmt"
	"maps"
	"text/template"

	sprig "github.com/go-task/slim-sprig/v3"

	"github.com/idelchi/godyl/pkg/path/file"

	"go.yaml.in/yaml/v4"
)

// Load reads a YAML file, expanding Go templates before parsing, and returns a list of strings.
func Load(path string, vars map[string]string) ([]string, error) {
	data, err := file.New(path).Read()
	if err != nil {
		return nil, fmt.Errorf("reading tmpl file: %w", err)
	}

	return parse(data, vars)
}

// parse expands Go templates in raw bytes, then unmarshals YAML into a string slice.
func parse(data []byte, vars map[string]string) ([]string, error) {
	expanded, err := Expand(data, vars)
	if err != nil {
		return nil, fmt.Errorf("expanding tmpl template: %w", err)
	}

	var r []string
	if err := yaml.Unmarshal(expanded, &r); err != nil {
		return nil, fmt.Errorf("parsing YAML (expected list of strings): %w", err)
	}

	return r, nil
}

// Expand executes data as a Go template with sprig functions and merged variables.
// Pass nil for vars when no overrides are needed; use sprig's {{ env "VAR" }} for env vars.
func Expand(data []byte, vars map[string]string) ([]byte, error) {
	ctx := buildContext(vars)

	tmpl, err := template.New("tmpl").
		Funcs(sprig.FuncMap()).
		Option("missingkey=zero").
		Parse(string(data))
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// buildContext returns the template context from --set overrides.
// For env vars, use sprig's {{ env "VAR" }} in templates.
func buildContext(vars map[string]string) map[string]string {
	ctx := make(map[string]string, len(vars))

	maps.Copy(ctx, vars)

	return ctx
}
