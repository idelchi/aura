package tool

import (
	"context"
	"testing"
)

func TestResolvePath(t *testing.T) {
	t.Parallel()

	ctx := WithWorkDir(context.Background(), "/home/user/project")

	tests := []struct {
		name string
		path string
		want string
	}{
		{"absolute unchanged", "/etc/hostname", "/etc/hostname"},
		{"relative joins with workdir", "src/main.go", "/home/user/project/src/main.go"},
		{"leading space trimmed", " /etc/hostname", "/etc/hostname"},
		{"trailing space trimmed", "/etc/hostname ", "/etc/hostname"},
		{"both spaces trimmed", "  /etc/hostname  ", "/etc/hostname"},
		{"leading space relative", " src/main.go", "/home/user/project/src/main.go"},
		{"interior spaces preserved", "/path/with spaces/file.txt", "/path/with spaces/file.txt"},
		{"whitespace-only resolves to workdir", "   ", "/home/user/project"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ResolvePath(ctx, tt.path)
			if got != tt.want {
				t.Errorf("ResolvePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestResolvePath_NoWorkDir(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	got := ResolvePath(ctx, " relative/path.go")
	if got != "relative/path.go" {
		t.Errorf("ResolvePath without workdir = %q, want %q", got, "relative/path.go")
	}
}
