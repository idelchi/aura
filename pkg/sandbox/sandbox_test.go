package sandbox_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/idelchi/aura/pkg/sandbox"
)

// mkDir creates a directory under root. Fatals on error.
func mkDir(t *testing.T, root, rel string) string {
	t.Helper()

	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(abs, 0o755); err != nil {
		t.Fatal(err)
	}

	return abs
}

// mkFile creates an empty file (with parent dirs) under root. Fatals on error.
func mkFile(t *testing.T, root, rel string) string {
	t.Helper()

	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(abs, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	return abs
}

func TestCanRead(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T, root string, s *sandbox.Sandbox)
		query string
		want  bool
	}{
		{
			name: "file under read-only folder",
			setup: func(t *testing.T, root string, s *sandbox.Sandbox) {
				s.AddReadOnly(mkDir(t, root, "data"))
				mkFile(t, root, "data/notes.txt")
			},
			query: "data/notes.txt",
			want:  true,
		},
		{
			name: "file under read-write folder is also readable",
			setup: func(t *testing.T, root string, s *sandbox.Sandbox) {
				s.AddReadWrite(mkDir(t, root, "ws"))
				mkFile(t, root, "ws/doc.md")
			},
			query: "ws/doc.md",
			want:  true,
		},
		{
			name: "exact read-only file",
			setup: func(t *testing.T, root string, s *sandbox.Sandbox) {
				s.AddReadOnly(mkFile(t, root, "config.yaml"))
			},
			query: "config.yaml",
			want:  true,
		},
		{
			name: "read-write file is also readable",
			setup: func(t *testing.T, root string, s *sandbox.Sandbox) {
				s.AddReadWrite(mkFile(t, root, "state.db"))
			},
			query: "state.db",
			want:  true,
		},
		{
			name: "path outside all rules",
			setup: func(t *testing.T, root string, s *sandbox.Sandbox) {
				s.AddReadOnly(mkDir(t, root, "allowed"))
				mkFile(t, root, "forbidden/secret.txt")
			},
			query: "forbidden/secret.txt",
			want:  false,
		},
		{
			name: "deeply nested under read-only folder",
			setup: func(t *testing.T, root string, s *sandbox.Sandbox) {
				s.AddReadOnly(mkDir(t, root, "project"))
				mkFile(t, root, "project/src/pkg/deep.go")
			},
			query: "project/src/pkg/deep.go",
			want:  true,
		},
		{
			name: "folder path itself is readable",
			setup: func(t *testing.T, root string, s *sandbox.Sandbox) {
				s.AddReadOnly(mkDir(t, root, "logs"))
			},
			query: "logs",
			want:  true,
		},
		{
			name: "prefix overlap does not grant access",
			setup: func(t *testing.T, root string, s *sandbox.Sandbox) {
				s.AddReadOnly(mkDir(t, root, "projects"))
				mkFile(t, root, "projects-evil/hack.sh")
			},
			query: "projects-evil/hack.sh",
			want:  false,
		},
		{
			name: "empty sandbox denies everything",
			setup: func(t *testing.T, root string, _ *sandbox.Sandbox) {
				mkFile(t, root, "anything.txt")
			},
			query: "anything.txt",
			want:  false,
		},
		{
			name: "nonexistent query path under valid folder",
			setup: func(t *testing.T, root string, s *sandbox.Sandbox) {
				s.AddReadOnly(mkDir(t, root, "data"))
			},
			query: "data/ghost.txt",
			want:  true, // policy check, not existence check
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			s := &sandbox.Sandbox{}
			tt.setup(t, root, s)

			got := s.CanRead(filepath.Join(root, tt.query))
			if got != tt.want {
				t.Errorf("CanRead(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestCanWrite(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T, root string, s *sandbox.Sandbox)
		query string
		want  bool
	}{
		{
			name: "file under read-write folder",
			setup: func(t *testing.T, root string, s *sandbox.Sandbox) {
				s.AddReadWrite(mkDir(t, root, "ws"))
				mkFile(t, root, "ws/output.log")
			},
			query: "ws/output.log",
			want:  true,
		},
		{
			name: "file under read-only folder is not writable",
			setup: func(t *testing.T, root string, s *sandbox.Sandbox) {
				s.AddReadOnly(mkDir(t, root, "readonly"))
				mkFile(t, root, "readonly/immutable.txt")
			},
			query: "readonly/immutable.txt",
			want:  false,
		},
		{
			name: "exact read-write file",
			setup: func(t *testing.T, root string, s *sandbox.Sandbox) {
				s.AddReadWrite(mkFile(t, root, "cache.db"))
			},
			query: "cache.db",
			want:  true,
		},
		{
			name: "exact read-only file is not writable",
			setup: func(t *testing.T, root string, s *sandbox.Sandbox) {
				s.AddReadOnly(mkFile(t, root, "config.yaml"))
			},
			query: "config.yaml",
			want:  false,
		},
		{
			name: "path outside all rules",
			setup: func(t *testing.T, root string, s *sandbox.Sandbox) {
				s.AddReadWrite(mkDir(t, root, "allowed"))
				mkFile(t, root, "other/file.txt")
			},
			query: "other/file.txt",
			want:  false,
		},
		{
			name: "deeply nested under read-write folder",
			setup: func(t *testing.T, root string, s *sandbox.Sandbox) {
				s.AddReadWrite(mkDir(t, root, "build"))
				mkFile(t, root, "build/out/bin/app")
			},
			query: "build/out/bin/app",
			want:  true,
		},
		{
			name: "prefix overlap does not grant write",
			setup: func(t *testing.T, root string, s *sandbox.Sandbox) {
				s.AddReadWrite(mkDir(t, root, "safe"))
				mkFile(t, root, "safe-not/payload.sh")
			},
			query: "safe-not/payload.sh",
			want:  false,
		},
		{
			name: "empty sandbox denies write",
			setup: func(t *testing.T, root string, _ *sandbox.Sandbox) {
				mkFile(t, root, "anything.txt")
			},
			query: "anything.txt",
			want:  false,
		},
		{
			name: "nonexistent path under writable folder",
			setup: func(t *testing.T, root string, s *sandbox.Sandbox) {
				s.AddReadWrite(mkDir(t, root, "output"))
			},
			query: "output/new-file.txt",
			want:  true,
		},
		{
			name: "read-only folder child not writable when sibling is read-write",
			setup: func(t *testing.T, root string, s *sandbox.Sandbox) {
				s.AddReadOnly(mkDir(t, root, "configs"))
				s.AddReadWrite(mkDir(t, root, "data"))
				mkFile(t, root, "configs/app.toml")
			},
			query: "configs/app.toml",
			want:  false,
		},
		{
			name: "same path in both read-only and read-write is writable",
			setup: func(t *testing.T, root string, s *sandbox.Sandbox) {
				dir := mkDir(t, root, "shared")
				s.AddReadOnly(dir)
				s.AddReadWrite(dir)
				mkFile(t, root, "shared/file.txt")
			},
			query: "shared/file.txt",
			want:  true,
		},
		{
			name: "file rule does not cover sibling",
			setup: func(t *testing.T, root string, s *sandbox.Sandbox) {
				s.AddReadWrite(mkFile(t, root, "dir/target.txt"))
				mkFile(t, root, "dir/sibling.txt")
			},
			query: "dir/sibling.txt",
			want:  false,
		},
		{
			name: "child folder rule does not leak to parent",
			setup: func(t *testing.T, root string, s *sandbox.Sandbox) {
				s.AddReadWrite(mkDir(t, root, "only-this"))
			},
			query: ".",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			s := &sandbox.Sandbox{}
			tt.setup(t, root, s)

			got := s.CanWrite(filepath.Join(root, tt.query))
			if got != tt.want {
				t.Errorf("CanWrite(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}
