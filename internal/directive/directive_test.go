package directive_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/directive"
	"github.com/idelchi/aura/pkg/image"
)

// writeFile writes content to path, creating the file if it does not exist.
func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeFile %s: %v", path, err)
	}
}

func TestParseNoDirectives(t *testing.T) {
	t.Parallel()

	input := "plain text no directives"
	result := directive.Parse(context.Background(), input, t.TempDir(), directive.Config{})

	if result.Text != input {
		t.Errorf("Text = %q, want %q", result.Text, input)
	}

	if len(result.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none", result.Warnings)
	}

	if result.HasImages() {
		t.Error("HasImages() = true, want false")
	}

	if result.Preamble != "" {
		t.Errorf("Preamble = %q, want empty", result.Preamble)
	}
}

func TestParseFileDirective(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	filename := "test.txt"
	writeFile(t, tmpDir+"/"+filename, "hello from file\nsecond line\n")

	result := directive.Parse(context.Background(), "@File["+filename+"]", tmpDir, directive.Config{})

	if len(result.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", result.Warnings)
	}

	wantToken := "[File: " + filename + "]"
	if result.Text != wantToken {
		t.Errorf("Text = %q, want %q", result.Text, wantToken)
	}

	if !strings.Contains(result.Preamble, "hello from file") {
		t.Errorf("Preamble does not contain file content; got: %q", result.Preamble)
	}
}

func TestParseFileMissing(t *testing.T) {
	t.Parallel()

	input := "@File[nonexistent.txt]"
	result := directive.Parse(context.Background(), input, t.TempDir(), directive.Config{})

	if len(result.Warnings) == 0 {
		t.Fatal("expected a warning for missing file, got none")
	}

	foundWarning := false

	for _, w := range result.Warnings {
		if strings.Contains(w, "file not found") {
			foundWarning = true

			break
		}
	}

	if !foundWarning {
		t.Errorf("no 'file not found' warning; warnings = %v", result.Warnings)
	}

	// Token must NOT be replaced when the file is missing.
	if result.Text != input {
		t.Errorf("Text = %q, want original token %q", result.Text, input)
	}
}

func TestParseFileDirectory(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	dirName := "mydir"
	childDir := tmpDir + "/" + dirName

	if err := os.MkdirAll(childDir, 0o755); err != nil {
		t.Fatalf("setup mkdir: %v", err)
	}

	for _, name := range []string{"alpha.txt", "beta.txt"} {
		writeFile(t, childDir+"/"+name, "content")
	}

	result := directive.Parse(context.Background(), "@File["+dirName+"]", tmpDir, directive.Config{})

	if len(result.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", result.Warnings)
	}

	wantToken := "[File: " + dirName + "]"
	if result.Text != wantToken {
		t.Errorf("Text = %q, want %q", result.Text, wantToken)
	}

	if !strings.Contains(result.Preamble, "Directory:") {
		t.Errorf("Preamble missing 'Directory:'; got: %q", result.Preamble)
	}

	for _, name := range []string{"alpha.txt", "beta.txt"} {
		if !strings.Contains(result.Preamble, name) {
			t.Errorf("Preamble missing filename %q; got: %q", name, result.Preamble)
		}
	}
}

func TestParseFileTruncation(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// 600 lines — above the maxFileLines=500 threshold.
	var sb strings.Builder

	for i := range 600 {
		fmt.Fprintf(&sb, "line %d\n", i+1)
	}

	filename := "big.txt"
	writeFile(t, tmpDir+"/"+filename, sb.String())

	result := directive.Parse(context.Background(), "@File["+filename+"]", tmpDir, directive.Config{})

	if len(result.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", result.Warnings)
	}

	if !strings.Contains(result.Preamble, "truncated at 500 lines") {
		t.Errorf("Preamble does not contain truncation notice; got: %q", result.Preamble)
	}
}

func TestParseBashDirective(t *testing.T) {
	t.Parallel()

	result := directive.Parse(context.Background(), "@Bash[echo hello]", t.TempDir(), directive.Config{
		RunBash: func(ctx context.Context, command string) (string, error) {
			out, err := exec.CommandContext(ctx, "sh", "-c", command).Output()

			return strings.TrimRight(string(out), "\n"), err
		},
	})

	wantToken := "[Command: echo hello]"
	if result.Text != wantToken {
		t.Errorf("Text = %q, want %q", result.Text, wantToken)
	}

	if !strings.Contains(result.Preamble, "hello") {
		t.Errorf("Preamble does not contain command output; got: %q", result.Preamble)
	}
}

func TestParseBashDirectiveNilCallback(t *testing.T) {
	t.Parallel()

	input := "@Bash[echo hello]"
	result := directive.Parse(context.Background(), input, t.TempDir(), directive.Config{})

	// Token must remain un-replaced when no executor is configured.
	if result.Text != input {
		t.Errorf("Text = %q, want original token %q", result.Text, input)
	}

	if len(result.Warnings) == 0 {
		t.Fatal("expected a warning for nil RunBash, got none")
	}

	if !strings.Contains(result.Warnings[0], "no bash executor configured") {
		t.Errorf("unexpected warning: %s", result.Warnings[0])
	}
}

func TestParsePathDirective(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "absolute path replaced bare",
			input: "@Path[/tmp/foo]",
			want:  "/tmp/foo",
		},
		{
			name:  "path with surrounding text",
			input: "@Path[/var/log] see logs",
			want:  "/var/log see logs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := directive.Parse(context.Background(), tt.input, t.TempDir(), directive.Config{})

			if result.Text != tt.want {
				t.Errorf("Text = %q, want %q", result.Text, tt.want)
			}

			if result.Preamble != "" {
				t.Errorf("Preamble = %q, want empty", result.Preamble)
			}

			if len(result.Warnings) != 0 {
				t.Errorf("Warnings = %v, want none", result.Warnings)
			}
		})
	}
}

func TestParsePathEnvExpand(t *testing.T) {
	// t.Parallel() omitted: t.Setenv cannot be used with t.Parallel() in this Go toolchain.
	t.Setenv("TEST_DIR_VAR", "/custom/path")

	result := directive.Parse(context.Background(), "@Path[$TEST_DIR_VAR/foo]", t.TempDir(), directive.Config{})

	want := "/custom/path/foo"
	if result.Text != want {
		t.Errorf("Text = %q, want %q", result.Text, want)
	}
}

func TestParseImageMissing(t *testing.T) {
	t.Parallel()

	input := "@Image[missing.png]"
	result := directive.Parse(context.Background(), input, t.TempDir(), directive.Config{})

	if len(result.Warnings) == 0 {
		t.Fatal("expected a warning for missing image, got none")
	}

	foundWarning := false

	for _, w := range result.Warnings {
		if strings.Contains(w, "file not found") {
			foundWarning = true

			break
		}
	}

	if !foundWarning {
		t.Errorf("no 'file not found' warning; warnings = %v", result.Warnings)
	}

	// Token must remain unchanged when the image is missing.
	if result.Text != input {
		t.Errorf("Text = %q, want original token %q", result.Text, input)
	}
}

func TestHasImages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		images image.Images
		want   bool
	}{
		{
			name:   "zero value is false",
			images: nil,
			want:   false,
		},
		{
			name:   "empty slice is false",
			images: image.Images{},
			want:   false,
		},
		{
			name:   "non-empty slice is true",
			images: image.Images{image.Image("fakedata")},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := directive.ParsedInput{Images: tt.images}
			if got := p.HasImages(); got != tt.want {
				t.Errorf("HasImages() = %v, want %v", got, tt.want)
			}
		})
	}
}
