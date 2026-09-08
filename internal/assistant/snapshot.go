package assistant

import (
	"fmt"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/snapshot"
	"github.com/idelchi/aura/internal/ui"
)

// ClearSnapshots removes this session's existing code checkpoints without
// initializing Git, including checkpoints created before snapshots were disabled.
func (a *Assistant) ClearSnapshots() error {
	if a.tools.snapshots == nil {
		return nil
	}

	return a.tools.snapshots.Prune()
}

// createSnapshot captures an accepted turn after all feature overlays have been
// resolved. Git initialization is lazy so disabled tasks do no snapshot work.
func (a *Assistant) createSnapshot(input string, messageIndex int) {
	if a.cfg.Features.Snapshot.IsDisabled() {
		return
	}

	dir := a.effectiveWorkDir()
	if a.tools.snapshotDir != dir {
		if err := a.ClearSnapshots(); err != nil {
			debug.Log("[snapshot] prune: %v", err)
		}

		a.tools.snapshots = snapshot.NewManager(dir)
		a.tools.snapshotDir = dir
	}

	if a.tools.snapshots == nil {
		return
	}

	a.send(ui.SpinnerMessage{Text: "Creating snapshot..."})
	if _, err := a.tools.snapshots.Create(input, messageIndex); err != nil {
		debug.Log("[snapshot] create failed: %v", err)
		a.send(ui.CommandResult{
			Message: fmt.Sprintf("warning: snapshot failed, /undo may be unavailable: %v", err),
			Level:   ui.LevelWarn,
		})
	}
}
