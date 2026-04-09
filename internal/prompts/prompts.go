package prompts

import (
	"bytes"
	"strings"
	"text/template"

	sprig "github.com/go-task/slim-sprig/v3"
)

// Prompt represents a text prompt with support for templating and composition.
type Prompt string

// Append adds content to the prompt with a newline separator.
func (p *Prompt) Append(content string) {
	*p += Prompt("\n" + content)
}

// String returns the prompt as a trimmed string.
func (p Prompt) String() string {
	return strings.TrimSpace(string(p))
}

// Render executes the prompt as a Go template with the given data.
func (p Prompt) Render(data any) (Prompt, error) {
	tmpl, err := template.New("prompt").Funcs(sprig.FuncMap()).Option("missingkey=error").Parse(p.String())
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return Prompt(buf.String()), nil
}
