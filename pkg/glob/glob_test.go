package glob_test

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/idelchi/aura/pkg/glob"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// buildTree creates a directory tree under dir and returns its absolute root.
// Files is a slice of slash-separated relative paths to create.
func buildTree(t *testing.T, dir string, paths []string) {
	t.Helper()

	for _, p := range paths {
		full := filepath.Join(dir, filepath.FromSlash(p))

		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", filepath.Dir(full), err)
		}

		if err := os.WriteFile(full, []byte{}, 0o600); err != nil {
			t.Fatalf("WriteFile(%q): %v", full, err)
		}
	}
}

func TestGlob(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		// wantRel contains the expected relative paths (slash-separated) of matched files.
		wantRel []string
		wantErr bool
	}{
		{
			name:    "top-level go files only",
			pattern: "*.go",
			wantRel: []string{"a.go"},
		},
		{
			name:    "txt files at any depth",
			pattern: "**/*.txt",
			wantRel: []string{"b.txt", "sub/d.txt"},
		},
		{
			name:    "no match returns empty slice",
			pattern: "*.rs",
			wantRel: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			buildTree(t, dir, []string{"a.go", "b.txt", "sub/c.go", "sub/d.txt"})

			root := folder.New(dir)
			matched, err := glob.Glob(root, tt.pattern)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Glob(%q) = nil, want error", tt.pattern)
				}

				return
			}

			if err != nil {
				t.Errorf("Glob(%q) unexpected error: %v", tt.pattern, err)

				return
			}

			// Convert absolute results back to relative paths for comparison.
			var gotRel []string

			for _, f := range matched {
				rel, err := filepath.Rel(dir, filepath.FromSlash(f.Path()))
				if err != nil {
					t.Fatalf("Rel(%q, %q): %v", dir, f.Path(), err)
				}

				gotRel = append(gotRel, filepath.ToSlash(rel))
			}

			// Normalise both slices before comparison.
			sort.Strings(gotRel)

			want := make([]string, len(tt.wantRel))
			copy(want, tt.wantRel)
			sort.Strings(want)

			if len(gotRel) != len(want) {
				t.Errorf("Glob(%q) matched %v, want %v", tt.pattern, gotRel, want)

				return
			}

			for i := range want {
				if gotRel[i] != want[i] {
					t.Errorf("Glob(%q) match[%d] = %q, want %q", tt.pattern, i, gotRel[i], want[i])
				}
			}
		})
	}
}
