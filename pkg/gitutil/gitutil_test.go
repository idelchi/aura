package gitutil_test

import (
	"testing"

	"github.com/idelchi/aura/pkg/gitutil"
)

func TestDeriveNameHTTPS(t *testing.T) {
	t.Parallel()

	got := gitutil.DeriveName("https://github.com/user/aura-plugin-metrics", "aura-plugin-")
	want := "metrics"

	if got != want {
		t.Errorf("DeriveName() = %q, want %q", got, want)
	}
}

func TestDeriveNameSSH(t *testing.T) {
	t.Parallel()

	got := gitutil.DeriveName("git@github.com:user/aura-plugin-metrics.git", "aura-plugin-")
	want := "metrics"

	if got != want {
		t.Errorf("DeriveName() = %q, want %q", got, want)
	}
}

func TestDeriveNameNoPrefix(t *testing.T) {
	t.Parallel()

	got := gitutil.DeriveName("https://github.com/user/my-hooks", "aura-plugin-")
	want := "my-hooks"

	if got != want {
		t.Errorf("DeriveName() = %q, want %q", got, want)
	}
}

func TestDeriveNameTrailingGit(t *testing.T) {
	t.Parallel()

	got := gitutil.DeriveName("https://github.com/user/aura-plugin-storage.git", "aura-plugin-")
	want := "storage"

	if got != want {
		t.Errorf("DeriveName() = %q, want %q", got, want)
	}
}

func TestIsGitURLHTTPS(t *testing.T) {
	t.Parallel()

	if !gitutil.IsGitURL("https://github.com/user/repo") {
		t.Errorf("IsGitURL(https URL) = false, want true")
	}
}

func TestIsGitURLSSH(t *testing.T) {
	t.Parallel()

	if !gitutil.IsGitURL("git@github.com:user/repo") {
		t.Errorf("IsGitURL(SSH URL) = false, want true")
	}
}

func TestIsGitURLLocalPath(t *testing.T) {
	t.Parallel()

	if gitutil.IsGitURL("./path/to/plugin") {
		t.Errorf("IsGitURL(local path) = true, want false")
	}
}

func TestShortCommit(t *testing.T) {
	t.Parallel()

	hash := "abcdef1234567890abcdef1234567890abcdef12"
	got := gitutil.ShortCommit(hash)
	want := "abcdef123456"

	if got != want {
		t.Errorf("ShortCommit(%q) = %q, want %q", hash, got, want)
	}
}

func TestShortCommitShort(t *testing.T) {
	t.Parallel()

	hash := "abcdef12"
	got := gitutil.ShortCommit(hash)

	if got != hash {
		t.Errorf("ShortCommit(%q) = %q, want %q (unchanged)", hash, got, hash)
	}
}

func TestWriteReadOrigin(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	origin := gitutil.Origin{
		URL:    "https://github.com/user/repo",
		Ref:    "main",
		Commit: "abcdef123456",
	}

	if err := gitutil.WriteOrigin(dir, origin); err != nil {
		t.Fatalf("WriteOrigin() error = %v", err)
	}

	got, err := gitutil.ReadOrigin(dir)
	if err != nil {
		t.Fatalf("ReadOrigin() error = %v", err)
	}

	if got.URL != origin.URL {
		t.Errorf("URL = %q, want %q", got.URL, origin.URL)
	}

	if got.Ref != origin.Ref {
		t.Errorf("Ref = %q, want %q", got.Ref, origin.Ref)
	}

	if got.Commit != origin.Commit {
		t.Errorf("Commit = %q, want %q", got.Commit, origin.Commit)
	}
}

func TestReadOriginMissing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	_, err := gitutil.ReadOrigin(dir)
	if err == nil {
		t.Errorf("ReadOrigin() on empty dir = nil error, want error")
	}
}
