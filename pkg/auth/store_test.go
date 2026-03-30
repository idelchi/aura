package auth_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/idelchi/aura/pkg/auth"
)

func TestSaveAndLoad(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	if err := auth.Save(dir, "anthropic", "sk-test-123"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	token, path, err := auth.Load("anthropic", dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if token != "sk-test-123" {
		t.Errorf("token = %q, want %q", token, "sk-test-123")
	}

	if path != filepath.Join(dir, "anthropic") {
		t.Errorf("path = %q, want %q", path, filepath.Join(dir, "anthropic"))
	}
}

func TestLoadMultipleDirs(t *testing.T) {
	t.Parallel()

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	// Only save in dir2
	if err := auth.Save(dir2, "openai", "sk-openai"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	token, path, err := auth.Load("openai", dir1, dir2)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if token != "sk-openai" {
		t.Errorf("token = %q, want %q", token, "sk-openai")
	}

	if path != filepath.Join(dir2, "openai") {
		t.Errorf("path = %q, want %q", path, filepath.Join(dir2, "openai"))
	}
}

func TestLoadFirstDirWins(t *testing.T) {
	t.Parallel()

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	if err := auth.Save(dir1, "provider", "first"); err != nil {
		t.Fatalf("Save dir1: %v", err)
	}

	if err := auth.Save(dir2, "provider", "second"); err != nil {
		t.Fatalf("Save dir2: %v", err)
	}

	token, _, err := auth.Load("provider", dir1, dir2)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if token != "first" {
		t.Errorf("token = %q, want %q (first dir wins)", token, "first")
	}
}

func TestLoadNotFound(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	_, _, err := auth.Load("nonexistent", dir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error = %v, want errors.Is(err, os.ErrNotExist)", err)
	}
}

func TestLoadTrimsWhitespace(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "provider")

	if err := os.WriteFile(path, []byte("mytoken\n  "), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	token, _, err := auth.Load("provider", dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if token != "mytoken" {
		t.Errorf("token = %q, want %q", token, "mytoken")
	}
}

func TestLoadSkipsEmpty(t *testing.T) {
	t.Parallel()

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	// Write empty (whitespace-only) file in dir1
	if err := os.WriteFile(filepath.Join(dir1, "provider"), []byte("  \n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Write real token in dir2
	if err := auth.Save(dir2, "provider", "real-token"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	token, path, err := auth.Load("provider", dir1, dir2)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if token != "real-token" {
		t.Errorf("token = %q, want %q", token, "real-token")
	}

	if path != filepath.Join(dir2, "provider") {
		t.Errorf("path = %q, want from dir2", path)
	}
}

func TestSaveCreatesDir(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	dir := filepath.Join(base, "nested", "auth")

	if err := auth.Save(dir, "provider", "token"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "provider")); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

func TestPath(t *testing.T) {
	t.Parallel()

	got := auth.Path("/home/user", "anthropic")

	want := filepath.Join("/home/user", "anthropic")
	if got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}
