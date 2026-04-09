package gitutil

import (
	"errors"
	"fmt"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/idelchi/godyl/pkg/path/folder"
)

// Clone clones a git repository into target with the auth chain.
// ref is optional (branch or tag name); empty means default branch.
func Clone(repoURL, target, ref string) error {
	methods := authChain(repoURL)

	opts := &git.CloneOptions{
		URL: repoURL,
	}

	if ref != "" {
		// Try as branch first; if clone fails, retry as tag.
		opts.ReferenceName = plumbing.NewBranchReferenceName(ref)
		opts.SingleBranch = true
	}

	var lastErr error

	for _, m := range methods {
		auth, err := m.fn(repoURL)
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", m.name, err)

			continue
		}

		opts.Auth = auth

		_, err = git.PlainClone(target, false, opts)
		if err != nil && ref != "" && opts.ReferenceName == plumbing.NewBranchReferenceName(ref) {
			// Branch didn't work — try as tag.
			opts.ReferenceName = plumbing.NewTagReferenceName(ref)

			folder.New(target).Remove()

			_, err = git.PlainClone(target, false, opts)
		}

		if err != nil {
			lastErr = fmt.Errorf("%s: %w", m.name, err)

			folder.New(target).Remove()

			continue
		}

		return nil
	}

	return fmt.Errorf("auth exhausted: %w", lastErr)
}

// Pull fetches and pulls the latest changes for an existing git repo.
// Returns the old and new commit hashes.
func Pull(dir, repoURL string) (oldCommit, newCommit string, err error) {
	repo, err := git.PlainOpen(dir)
	if err != nil {
		return "", "", fmt.Errorf("opening repo at %s: %w", dir, err)
	}

	head, err := repo.Head()
	if err != nil {
		return "", "", fmt.Errorf("reading HEAD: %w", err)
	}

	oldCommit = head.Hash().String()

	wt, err := repo.Worktree()
	if err != nil {
		return "", "", fmt.Errorf("getting worktree: %w", err)
	}

	methods := authChain(repoURL)

	var lastErr error

	for _, m := range methods {
		auth, aErr := m.fn(repoURL)
		if aErr != nil {
			lastErr = fmt.Errorf("%s: %w", m.name, aErr)

			continue
		}

		pErr := wt.Pull(&git.PullOptions{
			Auth:       auth,
			RemoteName: "origin",
		})

		if errors.Is(pErr, git.NoErrAlreadyUpToDate) {
			newHead, err := repo.Head()
			if err != nil {
				return "", "", fmt.Errorf("head after pull: %w", err)
			}

			return oldCommit, newHead.Hash().String(), nil
		}

		if pErr != nil {
			lastErr = fmt.Errorf("%s: %w", m.name, pErr)

			continue
		}

		newHead, err := repo.Head()
		if err != nil {
			return "", "", fmt.Errorf("head after pull: %w", err)
		}

		return oldCommit, newHead.Hash().String(), nil
	}

	return "", "", fmt.Errorf("pull: auth exhausted: %w", lastErr)
}

// HeadCommit returns the current HEAD commit hash of a git repo directory.
func HeadCommit(dir string) (string, error) {
	repo, err := git.PlainOpen(dir)
	if err != nil {
		return "", err
	}

	head, err := repo.Head()
	if err != nil {
		return "", err
	}

	return head.Hash().String(), nil
}
