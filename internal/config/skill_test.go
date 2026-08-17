package config_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/godyl/pkg/path/folder"
)

func TestDiscoverSkillFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	files := map[string]string{
		"README.md":                  "# Skill pack",
		"standalone.md":              "---\nname: standalone\ndescription: legacy skill\n---\nBody",
		"pack/SKILL.md":              "test",
		"pack/CHANGELOG.md":          "test",
		"pack/references/details.md": "test",
		"pack/nested/SKILL.md":       "test",
	}

	for path, content := range files {
		path = filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}

		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	discovered, err := config.DiscoverSkillFiles(folder.New(root))
	if err != nil {
		t.Fatalf("DiscoverSkillFiles: %v", err)
	}

	var got []string
	for _, file := range discovered {
		rel, err := filepath.Rel(root, file.Path())
		if err != nil {
			t.Fatalf("Rel: %v", err)
		}

		got = append(got, filepath.ToSlash(rel))
	}
	slices.Sort(got)

	want := []string{"pack/SKILL.md", "pack/nested/SKILL.md", "standalone.md"}
	if !slices.Equal(got, want) {
		t.Fatalf("files = %v, want %v", got, want)
	}
}
