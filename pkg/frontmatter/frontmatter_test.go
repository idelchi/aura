package frontmatter_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/idelchi/aura/pkg/frontmatter"
	"github.com/idelchi/godyl/pkg/path/file"
)

type testMeta struct {
	Title string `yaml:"title"`
}

func writeTemp(t *testing.T, content string) file.File {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}

	return file.New(path)
}

func TestLoad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		content   string
		wantTitle string
		wantBody  string
		wantErr   bool
	}{
		{
			name:      "with frontmatter",
			content:   "---\ntitle: hello\n---\nbody content",
			wantTitle: "hello",
			wantBody:  "body content",
		},
		{
			name:     "without frontmatter",
			content:  "just body text",
			wantBody: "just body text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := writeTemp(t, tt.content)

			var meta testMeta

			body, err := frontmatter.Load(f, &meta)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Load() = nil, want error")
				}

				return
			}

			if err != nil {
				t.Errorf("Load() unexpected error: %v", err)

				return
			}

			if meta.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", meta.Title, tt.wantTitle)
			}

			if body != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}

func TestLoadRaw(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		content     string
		wantYAML    string
		wantBody    string
		wantErr     bool
		wantNilYAML bool
	}{
		{
			name:     "with frontmatter returns raw YAML and body",
			content:  "---\ntitle: hello\n---\nbody content",
			wantYAML: "title: hello\n",
			wantBody: "body content",
		},
		{
			name:        "without frontmatter returns nil YAML and full body",
			content:     "just body text",
			wantNilYAML: true,
			wantBody:    "just body text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := writeTemp(t, tt.content)

			yamlBytes, body, err := frontmatter.LoadRaw(f)

			if tt.wantErr {
				if err == nil {
					t.Errorf("LoadRaw() = nil, want error")
				}

				return
			}

			if err != nil {
				t.Errorf("LoadRaw() unexpected error: %v", err)

				return
			}

			if tt.wantNilYAML {
				if yamlBytes != nil {
					t.Errorf("yamlBytes = %q, want nil", yamlBytes)
				}
			} else {
				if string(yamlBytes) != tt.wantYAML {
					t.Errorf("yamlBytes = %q, want %q", string(yamlBytes), tt.wantYAML)
				}
			}

			if body != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}
