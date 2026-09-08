package assistant

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/config/override"
	"github.com/idelchi/aura/internal/slash/commands"
	"github.com/idelchi/aura/internal/ui"
)

// snapshotGit runs Git only inside an isolated test repository.
func snapshotGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestSnapshotCreation covers default, disabled and non-Git turns without a model call.
func TestSnapshotCreation(t *testing.T) {
	for _, tc := range []struct {
		name     string // diagnostic subtest label
		git      bool   // create an empty Git repository
		disabled bool   // explicit task-level setting
	}{
		{name: "default", git: true},
		{name: "disabled", git: true, disabled: true},
		{name: "non-git"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := selectionAssistant(t)
			dir := t.TempDir()
			if tc.git {
				snapshotGit(t, dir, "init", "--quiet")
			}
			if err := os.WriteFile(filepath.Join(dir, "example.txt"), []byte("original"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := a.SetWorkDir(dir); err != nil {
				t.Fatal(err)
			}
			if tc.disabled {
				if err := a.MergeFeatures(config.Features{Snapshot: config.Snapshot{Disabled: new(true)}}); err != nil {
					t.Fatal(err)
				}
			}
			if a.tools.snapshots != nil || a.tools.snapshotDir != "" {
				t.Fatal("snapshot initialization occurred before task overrides settled")
			}
			events := make(chan ui.Event, 100)
			a.events = events
			a.createSnapshot("inspect logs", 1)
			want := tc.git && !tc.disabled
			if (a.SnapshotManager() != nil) != want || a.Status().Snapshots != want {
				t.Fatal("snapshot availability disagrees with feature and repository state")
			}
			if (len(events) > 0) != want {
				t.Fatal("unexpected snapshot spinner or error")
			}
			if tc.disabled && a.tools.snapshotDir != "" {
				t.Fatal("disabled snapshots performed Git initialization")
			}
			if tc.git {
				refs := snapshotGit(t, dir, "for-each-ref", "refs/aura/snapshots/", "--format=%(refname)")
				if (refs != "") != want {
					t.Fatalf("unexpected snapshot refs: %q", refs)
				}
			}
		})
	}
}

// TestSnapshotOverrides exercises normal global, agent, mode, task and CLI precedence.
func TestSnapshotOverrides(t *testing.T) {
	a := selectionAssistant(t)
	a.globalFeatures.Snapshot.Disabled = new(true)
	if err := a.RebuildState(); err != nil {
		t.Fatal(err)
	}
	if !a.Resolved().Features.Snapshot.IsDisabled() {
		t.Fatal("global disable lost")
	}
	agentKey, agentConfig := a.cfg.Agents.GetWithKey(a.agent.Name)
	agentConfig.Metadata.Features.Snapshot.Disabled = new(false)
	a.cfg.Agents[agentKey] = *agentConfig
	if err := a.RebuildState(); err != nil {
		t.Fatal(err)
	}
	if a.Resolved().Features.Snapshot.IsDisabled() {
		t.Fatal("agent could not re-enable snapshots")
	}
	modeKey, modeConfig := a.cfg.Modes.GetWithKey(a.agent.Mode)
	modeConfig.Metadata.Features.Snapshot.Disabled = new(true)
	a.cfg.Modes[modeKey] = *modeConfig
	if err := a.RebuildState(); err != nil {
		t.Fatal(err)
	}
	if !a.Resolved().Features.Snapshot.IsDisabled() {
		t.Fatal("mode could not disable snapshots")
	}
	if err := a.MergeFeatures(config.Features{Snapshot: config.Snapshot{Disabled: new(false)}}); err != nil {
		t.Fatal(err)
	}
	if a.Resolved().Features.Snapshot.IsDisabled() {
		t.Fatal("task could not re-enable snapshots")
	}
	var target config.OverrideTarget
	nodes, err := override.Cache(&target, []string{"features.snapshot.disabled=true"})
	if err != nil {
		t.Fatal(err)
	}
	a.overrideNodes = nodes
	if err := a.RebuildState(); err != nil {
		t.Fatal(err)
	}
	if !a.Resolved().Features.Snapshot.IsDisabled() {
		t.Fatal("CLI did not win over task")
	}
}

// TestSnapshotReenableAndUndo preserves checkpoint numbering and message-only rewind.
func TestSnapshotReenableAndUndo(t *testing.T) {
	a := selectionAssistant(t)
	dir := t.TempDir()
	snapshotGit(t, dir, "init", "--quiet")
	if err := a.SetWorkDir(dir); err != nil {
		t.Fatal(err)
	}
	a.builder.AddUserMessage(t.Context(), "first", 1)
	a.createSnapshot("first", 1)
	mgr := a.SnapshotManager()
	if mgr == nil {
		t.Fatal("default snapshot missing")
	}
	if err := a.MergeFeatures(config.Features{Snapshot: config.Snapshot{Disabled: new(true)}}); err != nil {
		t.Fatal(err)
	}
	events := make(chan ui.Event, 100)
	a.events = events
	a.createSnapshot("disabled", 2)
	if len(events) != 0 || a.SnapshotManager() != nil || a.Status().Snapshots {
		t.Fatal("disabling retained active snapshot behavior")
	}
	notice, err := commands.Undo().Execute(t.Context(), a)
	if err != nil || !strings.Contains(notice, "disabled") {
		t.Fatalf("undo notice=%q error=%v", notice, err)
	}
	picker, ok := (<-events).(ui.PickerOpen)
	if !ok || len(picker.Items) != 1 || picker.Items[0].Action.(ui.UndoSnapshot).Hash != "" {
		t.Fatal("disabled undo offered code restoration")
	}
	a.applyUIAction(t.Context(), ui.UndoExecute{Hash: "stale", Mode: "code"})
	result, ok := (<-events).(ui.CommandResult)
	if !ok || result.Error == nil || !strings.Contains(result.Error.Error(), "snapshots") {
		t.Fatal("stale code restore action was not rejected clearly")
	}
	if err := a.MergeFeatures(config.Features{Snapshot: config.Snapshot{Disabled: new(false)}}); err != nil {
		t.Fatal(err)
	}
	a.createSnapshot("reenabled", 2)
	refs, err := mgr.List()
	if err != nil || len(refs) != 2 || a.SnapshotManager() != mgr {
		t.Fatalf("reenabling lost checkpoint sequence: %v %v", refs, err)
	}
	if err := a.MergeFeatures(config.Features{Snapshot: config.Snapshot{Disabled: new(true)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := commands.Clear().Execute(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	if refs, err := mgr.List(); err != nil || len(refs) != 0 {
		t.Fatalf("/new retained old checkpoints while disabled: %v %v", refs, err)
	}
}
