//go:build !windows

package bash

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"

	"mvdan.cc/sh/v3/interp"
)

func sysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

func registerProcess(_ int) {}

func killProcessGroup(pid int, signal syscall.Signal) error {
	return syscall.Kill(-pid, signal)
}

func newCommand(path string, args []string, hc interp.HandlerContext) *exec.Cmd {
	return &exec.Cmd{
		Path:        path,
		Args:        args,
		Env:         execEnv(hc.Env),
		Dir:         hc.Dir,
		Stdin:       hc.Stdin,
		Stdout:      hc.Stdout,
		Stderr:      hc.Stderr,
		SysProcAttr: sysProcAttr(),
	}
}

func handleExecError(ctx context.Context, hc interp.HandlerContext, err error) error {
	switch err := err.(type) {
	case *exec.ExitError:
		if status, ok := err.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			return interp.ExitStatus(128 + int(status.Signal()))
		}

		return interp.ExitStatus(err.ExitCode())
	case *exec.Error:
		fmt.Fprintf(hc.Stderr, "%v\n", err)

		return interp.ExitStatus(127)
	default:
		return err
	}
}
