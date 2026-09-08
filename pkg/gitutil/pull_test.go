package gitutil_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/idelchi/aura/pkg/gitutil"
)

// TestPullInstalledBranch verifies updates follow dev, not a divergent remote HEAD.
func TestPullInstalledBranch(t *testing.T) {
	t.Parallel()
	origin := t.TempDir()
	repo, err := git.PlainInitWithOptions(origin, &git.PlainInitOptions{InitOptions: git.InitOptions{DefaultBranch: "refs/heads/main"}})
	if err != nil {
		t.Fatal(err)
	}
	work, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	commit := func(dir, content string, wt *git.Worktree) string {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "value"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := wt.Add("value"); err != nil {
			t.Fatal(err)
		}
		h, err := wt.Commit(content, &git.CommitOptions{Author: &object.Signature{Name: "test", Email: "test@example.invalid", When: time.Now()}})
		if err != nil {
			t.Fatal(err)
		}
		return h.String()
	}
	commit(origin, "base", work)
	dev := plumbing.NewBranchReferenceName("dev")
	if err := work.Checkout(&git.CheckoutOptions{Branch: dev, Create: true}); err != nil {
		t.Fatal(err)
	}
	want := commit(origin, "dev one", work)
	if err := work.Checkout(&git.CheckoutOptions{Branch: "refs/heads/main"}); err != nil {
		t.Fatal(err)
	}
	commit(origin, "main only", work)
	clone := filepath.Join(t.TempDir(), "clone")
	if err := gitutil.Clone(origin, clone, "dev"); err != nil {
		t.Fatal(err)
	}
	old, current, err := gitutil.Pull(clone, origin)
	if err != nil || old != want || current != want {
		t.Fatalf("no-op dev pull: %s %s %v", old, current, err)
	}
	if err := work.Checkout(&git.CheckoutOptions{Branch: dev}); err != nil {
		t.Fatal(err)
	}
	next := commit(origin, "dev two", work)
	_, current, err = gitutil.Pull(clone, origin)
	if err != nil || current != next {
		t.Fatalf("advancing dev pull: %s %v", current, err)
	}
	local, err := git.PlainOpen(clone)
	if err != nil {
		t.Fatal(err)
	}
	localWork, err := local.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	commit(clone, "local divergence", localWork)
	commit(origin, "remote divergence", work)
	_, _, err = gitutil.Pull(clone, origin)
	if err == nil || strings.Contains(err.Error(), "auth exhausted") {
		t.Fatalf("must expose divergence, not authentication: %v", err)
	}
}
