//go:build !windows

package serve

import (
	"context"
	"fmt"
	"os"
	"syscall"
)

// watchLease holds the server on a launcher lease: the read end of a pipe whose write end
// the launcher keeps. One byte releases the server, which then outlives the launcher;
// end-of-file before that (the launcher died) cancels with ErrLauncherLost.
// The descriptor is marked close-on-exec first, so no tool or MCP child inherits it.
func watchLease(fd int, lose context.CancelCauseFunc) error {
	if fd < 3 {
		return fmt.Errorf("--launcher-fd %d must name an inherited descriptor (3 or above)", fd)
	}

	var stat syscall.Stat_t
	if err := syscall.Fstat(fd, &stat); err != nil {
		return fmt.Errorf("--launcher-fd %d: %w", fd, err)
	}

	syscall.CloseOnExec(fd)

	lease := os.NewFile(uintptr(fd), "launcher-lease")

	go func() {
		defer lease.Close()

		var b [1]byte
		if n, _ := lease.Read(b[:]); n == 1 {
			return
		}

		lose(ErrLauncherLost)
	}()

	return nil
}
