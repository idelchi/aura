package tmpl_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/idelchi/aura/internal/tmpl"
)

// writeFile is a test helper that writes content to a file in dir and returns the path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writeFile: %v", err)
	}

	return path
}

func TestExpand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		vars    map[string]string
		want    string
		wantErr bool
	}{
		{
			name:  "simple variable substitution",
			input: `{{ .FOO }}`,
			vars:  map[string]string{"FOO": "bar"},
			want:  "bar",
		},
		{
			name:  "missing variable renders empty string",
			input: `{{ .MISSING }}`,
			vars:  map[string]string{},
			want:  "",
		},
		{
			name:  "nil vars map is safe",
			input: `{{ .MISSING }}`,
			vars:  nil,
			want:  "",
		},
		{
			name:  "sprig upper function",
			input: `{{ upper "hello" }}`,
			vars:  nil,
			want:  "HELLO",
		},
		{
			name:  "sprig trim function",
			input: `{{ trim "  spaces  " }}`,
			vars:  nil,
			want:  "spaces",
		},
		{
			name:  "variable used inside string literal",
			input: `prefix-{{ .NAME }}-suffix`,
			vars:  map[string]string{"NAME": "world"},
			want:  "prefix-world-suffix",
		},
		{
			name:  "multiple variables",
			input: `{{ .A }} and {{ .B }}`,
			vars:  map[string]string{"A": "one", "B": "two"},
			want:  "one and two",
		},
		{
			name:  "empty input returns empty output",
			input: ``,
			vars:  nil,
			want:  "",
		},
		{
			name:    "template syntax error returns error",
			input:   `{{ invalid`,
			vars:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := tmpl.Expand([]byte(tt.input), tt.vars)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expand() error = nil, want non-nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("Expand() unexpected error: %v", err)
			}

			if string(got) != tt.want {
				t.Errorf("Expand() = %q, want %q", string(got), tt.want)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	t.Parallel()

	t.Run("valid YAML list", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := writeFile(t, dir, "cmds.yaml", "- \"cmd1\"\n- \"cmd2\"\n")

		got, err := tmpl.Load(path, nil)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if len(got) != 2 {
			t.Fatalf("len(got) = %d, want 2", len(got))
		}

		if got[0] != "cmd1" {
			t.Errorf("got[0] = %q, want %q", got[0], "cmd1")
		}

		if got[1] != "cmd2" {
			t.Errorf("got[1] = %q, want %q", got[1], "cmd2")
		}
	})

	t.Run("template variables expanded before YAML parse", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := writeFile(t, dir, "cmds.yaml", "- \"hello {{ .NAME }}\"\n")

		got, err := tmpl.Load(path, map[string]string{"NAME": "world"})
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if len(got) != 1 {
			t.Fatalf("len(got) = %d, want 1", len(got))
		}

		if got[0] != "hello world" {
			t.Errorf("got[0] = %q, want %q", got[0], "hello world")
		}
	})

	t.Run("invalid YAML returns error", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		// A YAML mapping is not a list of strings.
		path := writeFile(t, dir, "cmds.yaml", "not: a: list\n")

		_, err := tmpl.Load(path, nil)
		if err == nil {
			t.Errorf("Load() error = nil, want non-nil for invalid YAML")
		}
	})

	t.Run("missing file returns error", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "does_not_exist.yaml")

		_, err := tmpl.Load(path, nil)
		if err == nil {
			t.Errorf("Load() error = nil, want non-nil for missing file")
		}
	})

	t.Run("single entry list", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := writeFile(t, dir, "cmds.yaml", "- \"only one\"\n")

		got, err := tmpl.Load(path, nil)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if len(got) != 1 {
			t.Fatalf("len(got) = %d, want 1", len(got))
		}

		if got[0] != "only one" {
			t.Errorf("got[0] = %q, want %q", got[0], "only one")
		}
	})

	t.Run("template syntax error returns error", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := writeFile(t, dir, "cmds.yaml", "- \"{{ invalid\"\n")

		_, err := tmpl.Load(path, nil)
		if err == nil {
			t.Errorf("Load() error = nil, want non-nil for template syntax error")
		}
	})
}
