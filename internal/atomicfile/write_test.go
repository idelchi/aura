package atomicfile_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/idelchi/aura/internal/atomicfile"
)

func TestWriteFilePublishesCompleteReplacement(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshot.json")
	for _, data := range [][]byte{bytes.Repeat([]byte("old"), 4096), []byte("new"), {}} {
		if err := atomicfile.WriteFile(path, data); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, data) {
			t.Fatalf("read replacement: got %d bytes, want %d, error %v", len(got), len(data), err)
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 1 || entries[0].Name() != "snapshot.json" {
			t.Fatalf("temporary file left after publication: %v, %v", entries, err)
		}
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("private file mode = %v", info.Mode().Perm())
		}
	}
}

func TestWriteFileFailurePreservesDestinationAndCleansTemporaryFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "occupied")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(path, "keep")
	if err := os.WriteFile(sentinel, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := atomicfile.WriteFile(path, []byte("replacement")); err == nil {
		t.Fatal("replaced a non-empty directory")
	}
	got, err := os.ReadFile(sentinel)
	if err != nil || string(got) != "untouched" {
		t.Fatalf("failed replacement changed destination: %q, %v", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "occupied" {
		t.Fatalf("temporary file left after rejection: %v, %v", entries, err)
	}
}
