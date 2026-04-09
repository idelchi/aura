package snapshot

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/pkg/truncate"
	"github.com/idelchi/godyl/pkg/env"
	"github.com/idelchi/godyl/pkg/path/file"
)

// Snapshot represents a point-in-time working tree capture.
type Snapshot struct {
	// Ref is the git ref name (e.g. "refs/aura/snapshots/<pid>/0001").
	Ref string

	// Hash is the full commit SHA.
	Hash string

	// Message is the user's input text that triggered this turn.
	Message string

	// CreatedAt is when the snapshot was taken.
	CreatedAt time.Time

	// MessageIndex is the conversation message count at snapshot time.
	// Used for "messages only" rewind — truncate history to this index.
	MessageIndex int
}

// Manager handles snapshot lifecycle within a git repository.
type Manager struct {
	// repoDir is the working directory (must be inside a git repo).
	repoDir string

	// sequence is an auto-incrementing counter for ref naming within a session.
	sequence int

	// emptyRepo is true when the repo has no commits (no HEAD).
	// Stays true for the session — snapshots use their own refs, not HEAD.
	emptyRepo bool

	// prefix is the per-process ref namespace (e.g. "refs/aura/snapshots/<pid>").
	// Scoped by PID to prevent collisions between concurrent aura instances.
	prefix string
}

// NewManager creates a snapshot manager for the given directory.
// Returns nil if the directory is not inside a git repository.
func NewManager(dir string) *Manager {
	if !isGitRepo(dir) {
		debug.Log("[snapshot] %s is not a git repo, disabled", dir)

		return nil
	}

	empty := !hasHead(dir)
	debug.Log("[snapshot] initialized: dir=%s emptyRepo=%v", dir, empty)

	return &Manager{
		repoDir:   dir,
		emptyRepo: empty,
		prefix:    fmt.Sprintf("refs/aura/snapshots/%d", os.Getpid()),
	}
}

// Create captures the current working tree state as a snapshot.
// The message is the user's input text for display in the picker.
// messageIndex is the builder.Len() before user messages are added.
func (m *Manager) Create(message string, messageIndex int) (*Snapshot, error) {
	m.sequence++

	// 1. Create temp index file — then remove it so git creates a valid index from scratch.
	tmpIdx, err := file.CreateRandomInDir("", "aura-snapshot-idx-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp index: %w", err)
	}

	tmpIdxPath := tmpIdx.Path()

	tmpIdx.Remove()
	defer tmpIdx.Remove()

	e := env.FromEnv()
	e.AddPair("GIT_INDEX_FILE", tmpIdxPath)
	e.AddPair("GIT_AUTHOR_NAME", "aura")
	e.AddPair("GIT_AUTHOR_EMAIL", "aura@snapshot")
	e.AddPair("GIT_COMMITTER_NAME", "aura")
	e.AddPair("GIT_COMMITTER_EMAIL", "aura@snapshot")

	// 2. Read current HEAD into temp index (skip for empty repos — no HEAD to read)
	if !m.emptyRepo {
		if err := m.GitEnv(e.AsSlice(), "read-tree", "HEAD"); err != nil {
			return nil, fmt.Errorf("read-tree HEAD: %w", err)
		}
	}

	// 3. Add all working tree files (respects .gitignore)
	if err := m.GitEnv(e.AsSlice(), "add", "--all"); err != nil {
		return nil, fmt.Errorf("git add --all: %w", err)
	}

	// 4. Write tree object
	tree, err := m.GitOutputEnv(e.AsSlice(), "write-tree")
	if err != nil {
		return nil, fmt.Errorf("write-tree: %w", err)
	}

	// 5. Create commit object (root commit for empty repos, child of HEAD otherwise)
	now := time.Now()
	commitMsg := fmt.Sprintf(
		"aura snapshot %04d [idx=%d]: %s",
		m.sequence,
		messageIndex,
		truncate.Truncate(message, 72),
	)

	var hash string

	if m.emptyRepo {
		hash, err = m.GitOutputEnv(e.AsSlice(), "commit-tree", tree, "-m", commitMsg)
	} else {
		hash, err = m.GitOutputEnv(e.AsSlice(), "commit-tree", tree, "-m", commitMsg, "-p", "HEAD")
	}

	if err != nil {
		debug.Log("[snapshot] commit-tree failed: tree=%s emptyRepo=%v err=%v", tree, m.emptyRepo, err)

		return nil, fmt.Errorf("commit-tree: %w", err)
	}

	// 6. Store as named ref
	ref := fmt.Sprintf("%s/%04d", m.prefix, m.sequence)
	if err := m.Git("update-ref", ref, hash); err != nil {
		return nil, fmt.Errorf("update-ref: %w", err)
	}

	debug.Log("[snapshot] created %s (%s)", ref, hash[:8])

	return &Snapshot{
		Ref:          ref,
		Hash:         hash,
		Message:      message,
		CreatedAt:    now,
		MessageIndex: messageIndex,
	}, nil
}

// List returns all snapshots in chronological order (oldest first).
func (m *Manager) List() ([]Snapshot, error) {
	output, err := m.GitOutput("for-each-ref",
		m.prefix+"/",
		"--format=%(refname)\t%(objectname)\t%(creatordate:unix)\t%(subject)",
		"--sort=creatordate",
	)
	if err != nil {
		return nil, fmt.Errorf("listing snapshots: %w", err)
	}

	if output == "" {
		return nil, nil
	}

	var snapshots []Snapshot

	for line := range strings.SplitSeq(output, "\n") {
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) < 4 {
			continue
		}

		var ts time.Time

		var epoch int64
		if n, err := fmt.Sscanf(parts[2], "%d", &epoch); n == 1 && err == nil {
			ts = time.Unix(epoch, 0)
		}

		// Parse commit subject: "aura snapshot NNNN [idx=M]: <message>"
		subject := parts[3]
		msg := subject

		var msgIdx int

		if _, after, ok := strings.Cut(subject, ": "); ok {
			msg = after
		}

		// Extract MessageIndex from "[idx=N]" in the prefix
		if idxStart := strings.Index(subject, "[idx="); idxStart != -1 {
			if _, err := fmt.Sscanf(subject[idxStart:], "[idx=%d]", &msgIdx); err != nil {
				debug.Log("[snapshot] malformed idx tag in %q: %v", subject, err)
			}
		}

		snapshots = append(snapshots, Snapshot{
			Ref:          parts[0],
			Hash:         parts[1],
			Message:      msg,
			CreatedAt:    ts,
			MessageIndex: msgIdx,
		})
	}

	return snapshots, nil
}

// RestoreCode restores the working tree to the state captured in the snapshot.
// Does NOT modify the user's staging area or branch.
func (m *Manager) RestoreCode(hash string) error {
	// 1. Get file list from snapshot
	snapFiles, err := m.GitOutput("ls-tree", "-r", "--name-only", hash)
	if err != nil {
		return fmt.Errorf("ls-tree snapshot: %w", err)
	}

	snapSet := toSet(strings.Split(snapFiles, "\n"))

	// 2. Get current worktree file list (tracked + untracked, minus ignored)
	tracked, err := m.GitOutput("ls-files")
	if err != nil {
		return fmt.Errorf("ls-files tracked: %w", err)
	}

	untracked, err := m.GitOutput("ls-files", "--others", "--exclude-standard")
	if err != nil {
		return fmt.Errorf("ls-files untracked: %w", err)
	}

	var currentFiles []string

	if tracked != "" {
		currentFiles = append(currentFiles, strings.Split(tracked, "\n")...)
	}

	if untracked != "" {
		currentFiles = append(currentFiles, strings.Split(untracked, "\n")...)
	}

	// 3. Delete files that exist now but didn't exist in the snapshot
	for _, f := range currentFiles {
		if f == "" {
			continue
		}

		if !snapSet[f] {
			file.New(m.repoDir, f).Remove()
		}
	}

	// 4. Overwrite all files from snapshot using temp index — then remove so git creates from scratch.
	restoreIdx, err := file.CreateRandomInDir("", "aura-restore-idx-*")
	if err != nil {
		return fmt.Errorf("creating temp index: %w", err)
	}

	tmpIdxPath := restoreIdx.Path()

	restoreIdx.Remove()
	defer restoreIdx.Remove()

	e := env.FromEnv()
	e.AddPair("GIT_INDEX_FILE", tmpIdxPath)

	if err := m.GitEnv(e.AsSlice(), "read-tree", hash); err != nil {
		return fmt.Errorf("read-tree for restore: %w", err)
	}

	if err := m.GitEnv(e.AsSlice(), "checkout-index", "-a", "-f"); err != nil {
		return fmt.Errorf("checkout-index: %w", err)
	}

	return nil
}

// Prune removes all snapshot refs. Called on session end or /clear.
func (m *Manager) Prune() error {
	snapshots, err := m.List()
	if err != nil {
		return err
	}

	var errs []error

	for _, s := range snapshots {
		if err := m.Git("update-ref", "-d", s.Ref); err != nil {
			errs = append(errs, fmt.Errorf("prune %s: %w", s.Ref, err))
		}
	}

	return errors.Join(errs...)
}

// DiffStat returns a short diff stat between two snapshot hashes.
func (m *Manager) DiffStat(fromHash, toHash string) (string, error) {
	return m.GitOutput("diff", "--stat", fromHash, toHash)
}

// gitRun executes a git command in the repo directory.
// If env is non-nil, it overrides the process environment.
// If captureOutput is true, it returns trimmed stdout; otherwise returns "".
func (m *Manager) gitRun(env []string, captureOutput bool, args ...string) (string, error) {
	cmd := exec.Command("git", args...)

	cmd.Dir = m.repoDir

	if len(env) > 0 {
		cmd.Env = env
	}

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	if captureOutput {
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("git %s: %w (stderr: %s)", args[0], err, strings.TrimSpace(stderr.String()))
		}

		return strings.TrimSpace(string(out)), nil
	}

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w (stderr: %s)", args[0], err, strings.TrimSpace(stderr.String()))
	}

	return "", nil
}

// Git runs a git command in the repo directory.
func (m *Manager) Git(args ...string) error {
	_, err := m.gitRun(nil, false, args...)

	return err
}

// GitOutput runs a git command and returns trimmed stdout.
func (m *Manager) GitOutput(args ...string) (string, error) {
	return m.gitRun(nil, true, args...)
}

// GitEnv runs a git command with custom environment variables.
func (m *Manager) GitEnv(env []string, args ...string) error {
	_, err := m.gitRun(env, false, args...)

	return err
}

// GitOutputEnv runs a git command with custom env and returns trimmed stdout.
func (m *Manager) GitOutputEnv(env []string, args ...string) (string, error) {
	return m.gitRun(env, true, args...)
}

// isGitRepo checks if the given directory is inside a git repository.
func isGitRepo(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")

	cmd.Dir = dir

	return cmd.Run() == nil
}

// hasHead checks if HEAD resolves to a valid commit.
// Returns false for empty repos (git init with no commits).
func hasHead(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--verify", "HEAD")

	cmd.Dir = dir

	return cmd.Run() == nil
}

// toSet converts a string slice to a set (map[string]bool).
func toSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))

	for _, s := range ss {
		if s != "" {
			m[s] = true
		}
	}

	return m
}
