package prompts_test

import (
	"testing"

	"github.com/idelchi/aura/internal/prompts"
)

func TestPromptString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input prompts.Prompt
		want  string
	}{
		{
			name:  "trims leading and trailing whitespace",
			input: prompts.Prompt("  hello  "),
			want:  "hello",
		},
		{
			name:  "trims newlines",
			input: prompts.Prompt("\nhello\n"),
			want:  "hello",
		},
		{
			name:  "empty prompt",
			input: prompts.Prompt(""),
			want:  "",
		},
		{
			name:  "whitespace only",
			input: prompts.Prompt("   "),
			want:  "",
		},
		{
			name:  "no whitespace to trim",
			input: prompts.Prompt("hello"),
			want:  "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.input.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPromptAppend(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		initial prompts.Prompt
		appends []string
		want    string
	}{
		{
			name:    "single append joins with newline",
			initial: prompts.Prompt("line1"),
			appends: []string{"line2"},
			want:    "line1\nline2",
		},
		{
			name:    "multiple appends accumulate",
			initial: prompts.Prompt("line1"),
			appends: []string{"line2", "line3"},
			want:    "line1\nline2\nline3",
		},
		{
			name:    "append to empty prompt",
			initial: prompts.Prompt(""),
			appends: []string{"line1"},
			want:    "line1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := tt.initial
			for _, content := range tt.appends {
				p.Append(content)
			}

			got := p.String()
			if got != tt.want {
				t.Errorf("String() after Append = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPromptRenderSimple(t *testing.T) {
	t.Parallel()

	p := prompts.Prompt("Hello {{ .Name }}")

	got, err := p.Render(map[string]any{"Name": "world"})
	if err != nil {
		t.Fatalf("Render() unexpected error: %v", err)
	}

	want := "Hello world"
	if got.String() != want {
		t.Errorf("Render() = %q, want %q", got.String(), want)
	}
}

func TestPromptRenderSprig(t *testing.T) {
	t.Parallel()

	p := prompts.Prompt("{{ .Name | upper }}")

	got, err := p.Render(map[string]any{"Name": "hello"})
	if err != nil {
		t.Fatalf("Render() unexpected error: %v", err)
	}

	want := "HELLO"
	if got.String() != want {
		t.Errorf("Render() = %q, want %q", got.String(), want)
	}
}

func TestPromptRenderMissingKey(t *testing.T) {
	t.Parallel()

	// missingkey=error causes missing map keys to produce an error.
	p := prompts.Prompt("Hello {{ .Missing }}")

	_, err := p.Render(map[string]any{})
	if err == nil {
		t.Fatal("Render() expected error for missing map key with missingkey=error, got nil")
	}
}

func TestPromptRenderFrontmatter(t *testing.T) {
	t.Parallel()

	// Render() is a pure template executor — it does not strip frontmatter.
	// Frontmatter stripping happens at load time via frontmatter.Load().
	p := prompts.Prompt("---\nkey: val\n---\nBody {{ .X }}")

	got, err := p.Render(map[string]any{"X": "here"})
	if err != nil {
		t.Fatalf("Render() unexpected error: %v", err)
	}

	want := "---\nkey: val\n---\nBody here"
	if got.String() != want {
		t.Errorf("Render() = %q, want %q", got.String(), want)
	}
}

func TestPromptRenderInvalidTemplate(t *testing.T) {
	t.Parallel()

	p := prompts.Prompt("{{ .Name")

	_, err := p.Render(map[string]any{})
	if err == nil {
		t.Fatal("Render() expected error for invalid template, got nil")
	}
}
