package patch_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/tools/filetime"
	"github.com/idelchi/aura/internal/tools/patch"
	"github.com/idelchi/aura/pkg/llm/tool"
)

// testCtx returns a context carrying a fresh Tracker with default policy.
func testCtx(t *testing.T) (context.Context, *filetime.Tracker) {
	t.Helper()

	tracker := filetime.NewTracker(tool.DefaultReadBeforePolicy())

	return filetime.WithTracker(context.Background(), tracker), tracker
}

// executePatch is a convenience helper that calls Execute on a fresh Tool.
func executePatch(t *testing.T, ctx context.Context, patchContent string) (string, error) {
	t.Helper()

	tool := patch.New()

	return tool.Execute(ctx, map[string]any{"patch": patchContent})
}

func TestExecuteAddFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	newFile := filepath.Join(dir, "newfile.go")

	patchContent := fmt.Sprintf(
		"*** Begin Patch\n*** Add File: %s\n+package main\n+\n+func main() {}\n*** End Patch",
		newFile,
	)

	ctx, _ := testCtx(t)

	out, err := executePatch(t, ctx, patchContent)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(out, "Success") {
		t.Errorf("output = %q, want it to contain %q", out, "Success")
	}

	got, err := os.ReadFile(newFile)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v, file should have been created", newFile, err)
	}

	content := string(got)
	if !strings.Contains(content, "package main") {
		t.Errorf("file content = %q, want it to contain %q", content, "package main")
	}

	if !strings.Contains(content, "func main() {}") {
		t.Errorf("file content = %q, want it to contain %q", content, "func main() {}")
	}
}

func TestExecuteAddFileNestedDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	newFile := filepath.Join(dir, "sub", "deep", "newfile.go")

	patchContent := fmt.Sprintf(
		"*** Begin Patch\n*** Add File: %s\n+package main\n*** End Patch",
		newFile,
	)

	ctx, _ := testCtx(t)

	out, err := executePatch(t, ctx, patchContent)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(out, "Success") {
		t.Errorf("output = %q, want it to contain %q", out, "Success")
	}

	if _, err := os.Stat(newFile); os.IsNotExist(err) {
		t.Errorf("file %q was not created", newFile)
	}
}

func TestExecuteUpdateFileMoveToNestedDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	source := filepath.Join(dir, "old.go")
	dest := filepath.Join(dir, "new", "deep", "renamed.go")

	initial := "package main\n\nfunc hello() {}\n"
	if err := os.WriteFile(source, []byte(initial), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	ctx, tracker := testCtx(t)
	tracker.RecordRead(source)

	patchContent := fmt.Sprintf(
		"*** Begin Patch\n*** Update File: %s\n*** Move to: %s\n@@ func hello\n-func hello() {}\n+func hello() { return }\n*** End Patch",
		source,
		dest,
	)

	out, err := executePatch(t, ctx, patchContent)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(out, "Success") {
		t.Errorf("output = %q, want it to contain %q", out, "Success")
	}

	if _, err := os.Stat(dest); os.IsNotExist(err) {
		t.Errorf("destination file %q was not created", dest)
	}

	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Errorf("source file %q should have been removed after move", source)
	}
}

func TestExecuteUpdateFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "existing.go")

	initial := "package main\n\nfunc main() {\n    return nil\n}\n"
	if err := os.WriteFile(target, []byte(initial), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	ctx, tracker := testCtx(t)
	tracker.RecordRead(target)

	patchContent := fmt.Sprintf(
		"*** Begin Patch\n*** Update File: %s\n@@ func main\n-    return nil\n+    return 42\n*** End Patch",
		target,
	)

	out, err := executePatch(t, ctx, patchContent)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(out, "Success") {
		t.Errorf("output = %q, want it to contain %q", out, "Success")
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", target, err)
	}

	content := string(got)
	if strings.Contains(content, "return nil") {
		t.Errorf("file content = %q, old line %q should have been replaced", content, "return nil")
	}

	if !strings.Contains(content, "return 42") {
		t.Errorf("file content = %q, want it to contain %q", content, "return 42")
	}
}

func TestExecuteDeleteFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "todelete.txt")

	if err := os.WriteFile(target, []byte("bye\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	patchContent := fmt.Sprintf(
		"*** Begin Patch\n*** Delete File: %s\n*** End Patch",
		target,
	)

	ctx, _ := testCtx(t)

	out, err := executePatch(t, ctx, patchContent)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(out, "Success") {
		t.Errorf("output = %q, want it to contain %q", out, "Success")
	}

	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Errorf("file %q still exists after delete hunk", target)
	}
}

func TestExecuteMultipleHunks(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	newFile := filepath.Join(dir, "added.go")
	existingFile := filepath.Join(dir, "existing.go")

	initial := "package main\n\nfunc greet() {\n    return \"hello\"\n}\n"
	if err := os.WriteFile(existingFile, []byte(initial), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	ctx, tracker := testCtx(t)
	tracker.RecordRead(existingFile)

	patchContent := fmt.Sprintf(
		"*** Begin Patch\n"+
			"*** Add File: %s\n+package main\n+\n+func added() {}\n"+
			"*** Update File: %s\n@@ func greet\n-    return \"hello\"\n+    return \"world\"\n"+
			"*** End Patch",
		newFile,
		existingFile,
	)

	out, err := executePatch(t, ctx, patchContent)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(out, "Success") {
		t.Errorf("output = %q, want it to contain %q", out, "Success")
	}

	// Verify added file exists.
	if _, err := os.Stat(newFile); os.IsNotExist(err) {
		t.Errorf("added file %q was not created", newFile)
	}

	// Verify existing file was updated.
	got, err := os.ReadFile(existingFile)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", existingFile, err)
	}

	if !strings.Contains(string(got), "return \"world\"") {
		t.Errorf("file content = %q, want it to contain %q", string(got), `return "world"`)
	}
}

func TestExecuteUpdateFileNotFound(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	missing := filepath.Join(dir, "nonexistent.go")

	patchContent := fmt.Sprintf(
		"*** Begin Patch\n*** Update File: %s\n@@ func main\n-    return nil\n+    return 42\n*** End Patch",
		missing,
	)

	ctx, _ := testCtx(t)

	_, err := executePatch(t, ctx, patchContent)
	if err == nil {
		t.Fatal("Execute() error = nil, want non-nil for missing file")
	}
}

func TestExecuteSeekFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "source.go")

	// File content does NOT contain "return nil" — seek will fail.
	initial := "package main\n\nfunc main() {\n    fmt.Println(\"hello\")\n}\n"
	if err := os.WriteFile(target, []byte(initial), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	ctx, tracker := testCtx(t)
	tracker.RecordRead(target)

	patchContent := fmt.Sprintf(
		"*** Begin Patch\n*** Update File: %s\n@@ func main\n-    return nil\n+    return 42\n*** End Patch",
		target,
	)

	_, err := executePatch(t, ctx, patchContent)
	if err == nil {
		t.Fatal("Execute() error = nil, want non-nil when seek fails")
	}

	if !strings.Contains(err.Error(), "finding lines") {
		t.Errorf("err = %q, want it to mention %q", err.Error(), "finding lines")
	}
}
