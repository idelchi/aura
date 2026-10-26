//go:build !linux

package sandbox

import (
	"errors"
	"syscall"
)

// errUnsupported reports that confined launching needs Linux Landlock.
var errUnsupported = errors.New("sandboxed processes require Linux Landlock")

// childAttributes returns no special attributes; launching fails in ExecMain.
func childAttributes() *syscall.SysProcAttr {
	return nil
}

// replaceProcess is unsupported outside Linux.
func replaceProcess(_ string, _, _ []string) error {
	return errUnsupported
}

// KillGroup is unsupported outside Linux.
func KillGroup(_ int) error {
	return errUnsupported
}
