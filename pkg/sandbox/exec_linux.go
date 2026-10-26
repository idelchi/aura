package sandbox

import "syscall"

// childAttributes isolates the launcher in its own process group and kills it with its parent.
func childAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
}

// replaceProcess executes path in place of the current process; confinement is inherited.
func replaceProcess(path string, argv, env []string) error {
	return syscall.Exec(path, argv, env)
}

// KillGroup kills the process group led by pid.
func KillGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGKILL)
}
