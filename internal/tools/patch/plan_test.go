package patch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreflightFailureLeavesEarlierFilesUntouched(t *testing.T) {
	t.Parallel()
	for _, firstOperation := range []string{"update", "delete", "add"} {
		t.Run(firstOperation, func(t *testing.T) {
			dir := t.TempDir()
			first, second := filepath.Join(dir, "first"), filepath.Join(dir, "second")
			for _, path := range []string{first, second} {
				if err := os.WriteFile(path, []byte("original\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			operation := "*** Update File: " + first + "\n@@\n-original\n+changed\n"
			if firstOperation == "delete" {
				operation = "*** Delete File: " + first + "\n"
			}
			if firstOperation == "add" {
				operation = "*** Add File: " + first + "\n+overwritten\n"
			}
			input := "*** Begin Patch\n" + operation + "*** Update File: " + second + "\n@@\n-not in the file\n+bad\n*** End Patch"
			_, err := New().Execute(context.Background(), map[string]any{"patch": input})
			if err == nil {
				t.Fatal("expected later context failure")
			}
			for _, path := range []string{first, second} {
				got, err := os.ReadFile(path)
				if err != nil || string(got) != "original\n" {
					t.Fatalf("%s changed: %q, %v", path, got, err)
				}
			}
		})
	}
}

func TestPatchSupportsSequentialOperationsAndPreview(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	first, moved := filepath.Join(dir, "new file"), filepath.Join(dir, "nested", "moved file")
	input := fmt.Sprintf("*** Begin Patch\n*** Add File: %s\n+one\n*** Update File: %s\n*** Move to: %s\n@@\n-one\n+two\n*** Update File: %s\n@@\n-two\n+three\n*** Delete File: %s\n*** Add File: %s\n+final\n*** End Patch", first, first, moved, moved, moved, moved)
	args := map[string]any{"patch": input}
	tool := New()
	preview, err := tool.Preview(context.Background(), args)
	if err != nil || !strings.Contains(preview, "+final") {
		t.Fatalf("preview: %q, %v", preview, err)
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatal("preview changed the filesystem")
	}
	if _, err := os.Stat(moved); !os.IsNotExist(err) {
		t.Fatal("preview created destination")
	}
	output, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "R "+first+" -> "+moved) {
		t.Fatalf("missing move result: %s", output)
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatal("move left source")
	}
	got, err := os.ReadFile(moved)
	if err != nil || string(got) != "final" {
		t.Fatalf("destination: %q, %v", got, err)
	}
}

func TestApplyReportsCompletedChangesOnIOFailure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	first, second := filepath.Join(dir, "first"), filepath.Join(dir, "second")
	prepared, err := prepare(context.Background(), []Hunk{&AddFile{Path: first, Content: "created"}, &AddFile{Path: second, Content: "never written"}})
	if err != nil {
		t.Fatal(err)
	}
	// The filesystem changes after preflight; no preparation can promise a transaction.
	if err := os.Mkdir(second, 0o700); err != nil {
		t.Fatal(err)
	}
	_, err = prepared.Apply(context.Background())
	if err == nil || !strings.Contains(err.Error(), "already applied:\nA "+first) || !strings.Contains(err.Error(), second) {
		t.Fatalf("error: %v", err)
	}
	got, err := os.ReadFile(first)
	if err != nil || string(got) != "created" {
		t.Fatalf("first: %q, %v", got, err)
	}
}

func TestMissingLaterFileDoesNotDeleteEarlierFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "keep")
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	input := fmt.Sprintf("*** Begin Patch\n*** Delete File: %s\n*** Delete File: %s\n*** End Patch", path, path+"missing")
	if _, err := New().Execute(context.Background(), map[string]any{"patch": input}); err == nil {
		t.Fatal("missing deletion accepted")
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "keep" {
		t.Fatalf("earlier deletion applied: %q, %v", got, err)
	}
}

func TestPatchPreservesFilePermissions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	existing, added := filepath.Join(dir, "existing"), filepath.Join(dir, "added")
	if err := os.WriteFile(existing, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(existing, 0o750); err != nil {
		t.Fatal(err)
	}
	input := fmt.Sprintf("*** Begin Patch\n*** Update File: %s\n@@\n-old\n+new\n*** Add File: %s\n+new\n*** End Patch", existing, added)
	if _, err := New().Execute(context.Background(), map[string]any{"patch": input}); err != nil {
		t.Fatal(err)
	}
	for path, mode := range map[string]os.FileMode{existing: 0o750, added: 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != mode {
			t.Fatalf("%s: mode %o, want %o", path, info.Mode().Perm(), mode)
		}
	}
}
