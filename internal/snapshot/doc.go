// Package snapshot provides automatic working tree snapshots using git plumbing.
//
// Snapshots are stored as named refs under refs/aura/snapshots/<pid>/ in the
// user's repository, scoped by PID to prevent collisions between concurrent
// aura instances. Each snapshot captures the complete working tree state
// (tracked + untracked files, respecting .gitignore) without disturbing the
// user's staging area or branch history.
package snapshot
