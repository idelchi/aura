//go:build windows

package bash

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"mvdan.cc/sh/v3/interp"
)

func sysProcAttr() *syscall.SysProcAttr {
	return nil
}

func registerProcess(_ int) {}

func killProcessGroup(pid int, _ syscall.Signal) error {
	// On Windows, we can't use process groups the same way
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}

	return p.Kill()
}

func newCommand(path string, args []string, hc interp.HandlerContext) *exec.Cmd {
	return &exec.Cmd{
		Path:   path,
		Args:   args,
		Env:    execEnv(hc.Env),
		Dir:    hc.Dir,
		Stdin:  hc.Stdin,
		Stdout: hc.Stdout,
		Stderr: hc.Stderr,
	}
}

func handleExecError(ctx context.Context, hc interp.HandlerContext, err error) error {
	switch err := err.(type) {
	case *exec.ExitError:
		return interp.ExitStatus(err.ExitCode())
	case *exec.Error:
		fmt.Fprintf(hc.Stderr, "%v\n", err)

		return interp.ExitStatus(127)
	default:
		return err
	}
}
