// Package packs provides shared git-workflow operations for installable
// entities (plugins, skills) that can be sourced from git repos or local paths.
package packs

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/pkg/gitutil"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// CloneResult holds the outcome of a git clone operation.
type CloneResult struct {
	TargetDir string // absolute path of the cloned directory
	Commit    string // HEAD commit after clone (may be empty on error)
}

// PullResult holds the outcome of a git pull operation.
type PullResult struct {
	OldCommit string
	NewCommit string
	Changed   bool
}

// CleanSubpaths validates and normalizes --subpath flag values.
// Returns an error if any path is absolute or contains ".." components.
func CleanSubpaths(subpaths []string) ([]string, error) {
	var cleaned []string

	for _, sp := range subpaths {
		if sp == "" {
			continue
		}

		c := file.New(sp)
		if c.IsAbs() || strings.HasPrefix(c.Path(), "..") {
			return nil, fmt.Errorf("--subpath must be a relative path without '..' components: %q", sp)
		}

		cleaned = append(cleaned, c.Path())
	}

	return cleaned, nil
}

// DeriveName determines the install directory name from subpaths or source URL.
// Single subpath: uses the subpath's last segment (e.g., "plugins/injectors" → "injectors").
// Multiple subpaths or none: derives from the source URL via gitutil.DeriveName.
func DeriveName(source string, subpaths []string, prefix string) string {
	if len(subpaths) == 1 {
		return filepath.Base(subpaths[0])
	}

	return gitutil.DeriveName(source, prefix)
}

// CloneSource clones a git repo into targetDir/name, validates subpaths exist
// within the clone, reads HEAD commit, and returns the result.
//
// On clone or subpath failure, the target directory is cleaned up automatically.
// The caller is responsible for entity-specific validation after this returns.
// If entity validation fails, the caller should clean up via
// folder.New(result.TargetDir).Remove().
func CloneSource(
	w io.Writer,
	source, targetDir, name, ref string,
	subpaths []string,
	kind string,
) (CloneResult, error) {
	target := folder.New(targetDir, name)

	if target.Exists() {
		return CloneResult{}, fmt.Errorf(
			"%s %q already exists at %s\nUse 'aura %ss update %s' to update it",
			kind, name, target.Path(), kind, name,
		)
	}

	debug.Log("[%ss:add] cloning %s into %s (ref=%q)", kind, source, target.Path(), ref)
	fmt.Fprintf(w, "Cloning %s into %s...\n", source, target.Path())

	if err := gitutil.Clone(source, target.Path(), ref); err != nil {
		return CloneResult{}, fmt.Errorf("cloning: %w", err)
	}

	debug.Log("[%ss:add] clone done", kind)

	// Validate all subpaths exist within clone.
	for _, sp := range subpaths {
		scopedDir := folder.New(target.Path(), sp)
		if !scopedDir.Exists() {
			target.Remove()

			return CloneResult{}, fmt.Errorf("subpath %q does not exist in %s", sp, source)
		}
	}

	// Read HEAD commit.
	commit, err := gitutil.HeadCommit(target.Path())
	if err != nil {
		debug.Log("[%ss:add] head commit: %v", kind, err)
	}

	return CloneResult{
		TargetDir: target.Path(),
		Commit:    commit,
	}, nil
}

// WriteCloneOrigin writes the .origin.yaml sidecar for a freshly cloned source.
// Prints a warning on failure but does not return an error — the install succeeded,
// the origin is metadata.
func WriteCloneOrigin(w io.Writer, targetDir, source, ref, commit string, subpaths []string) {
	origin := gitutil.Origin{
		URL:      source,
		Ref:      ref,
		Commit:   commit,
		Subpaths: subpaths,
	}

	if err := gitutil.WriteOrigin(targetDir, origin); err != nil {
		fmt.Fprintf(w, "Warning: could not write origin: %v\n", err)
	}
}

// CopySource copies a local directory to targetDir/name, handling subpath scoping.
// Returns the absolute path of the created target directory.
//
// Note: callers that need to validate the source BEFORE copying (e.g., skill
// frontmatter validation) should scope the source by subpath themselves and pass
// the scoped path. The subpath parameter here handles the copy-time scoping — if
// the caller already scoped, the redundant scoping is harmless (same result path).
func CopySource(w io.Writer, source, targetDir, name, subpath, kind string) (string, error) {
	target := folder.New(targetDir, name)

	if target.Exists() {
		return "", fmt.Errorf("%s %q already exists at %s", kind, name, target.Path())
	}

	effectiveSource := source

	if subpath != "" {
		effectiveSource = folder.New(source, subpath).Path()

		if !folder.New(effectiveSource).Exists() {
			return "", fmt.Errorf("subpath %q does not exist in %s", subpath, source)
		}
	}

	debug.Log("[%ss:add-local] copying %s to %s", kind, effectiveSource, target.Path())
	fmt.Fprintf(w, "Copying %s to %s...\n", effectiveSource, target.Path())

	if err := gitutil.CopyDir(effectiveSource, target.Path()); err != nil {
		return "", fmt.Errorf("copying: %w", err)
	}

	return target.Path(), nil
}

// DiscoveryRoots returns the discovery root paths for the given subpaths within a target directory.
// If no subpaths are set, returns the target directory itself.
func DiscoveryRoots(targetDir string, subpaths []string) []string {
	if len(subpaths) == 0 {
		return []string{targetDir}
	}

	roots := make([]string, len(subpaths))
	for i, sp := range subpaths {
		roots[i] = folder.New(targetDir, sp).Path()
	}

	return roots
}

// PullUpdate pulls latest changes and returns the result.
//
// Prints "Updating X from Y..." before pulling and "Already up to date (hash)"
// when unchanged. Does NOT write origin or print "Updated" — the caller must do
// that after any entity-specific post-pull validation (e.g., plugin vendoring/SDK
// checks). This prevents stale origin commits when post-pull validation fails.
func PullUpdate(w io.Writer, originDir string, origin gitutil.Origin, label string) (PullResult, error) {
	debug.Log("[packs:update] pulling %s from %s", label, origin.URL)
	fmt.Fprintf(w, "Updating %s from %s...\n", label, origin.URL)

	oldCommit, newCommit, err := gitutil.Pull(originDir, origin.URL)
	if err != nil {
		return PullResult{}, fmt.Errorf("pulling %s: %w", label, err)
	}

	if oldCommit == newCommit {
		debug.Log("[packs:update] already up to date (%s)", gitutil.ShortCommit(oldCommit))
		fmt.Fprintf(w, "Already up to date (%s)\n", gitutil.ShortCommit(oldCommit))

		return PullResult{OldCommit: oldCommit, NewCommit: newCommit, Changed: false}, nil
	}

	debug.Log("[packs:update] updated %s → %s", gitutil.ShortCommit(oldCommit), gitutil.ShortCommit(newCommit))

	return PullResult{OldCommit: oldCommit, NewCommit: newCommit, Changed: true}, nil
}
