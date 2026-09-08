package tmpl

import (
	"bytes"
	"fmt"
	"text/template"

	sprig "github.com/go-task/slim-sprig/v3"
	"mvdan.cc/sh/v3/syntax"

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

// Expand executes data as a Go template with sprig functions and the supplied context.
// The context may be a variable map or structured runtime data. Use {{ env "VAR" }} for env vars.
func Expand(data []byte, vars any) ([]byte, error) {
	tmpl, err := template.New("tmpl").
		Funcs(sprig.FuncMap()).
		Funcs(template.FuncMap{"shellQuote": shellQuote}).
		Option("missingkey=zero").
		Parse(string(data))
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer

	if err := tmpl.Execute(&buf, vars); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// shellQuote preserves a template value as one literal word in the task shell.
func shellQuote(value string) (string, error) {
	return syntax.Quote(value, syntax.LangBash)
}
