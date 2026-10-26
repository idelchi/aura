package sandbox

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// ExecArg is the private first argument that turns the aura binary into a confining launcher.
// main dispatches it before any CLI parsing, configuration or env-file loading.
const ExecArg = "__aura-sandbox-exec"

// ExecSpec is the confinement a launcher applies before executing its command.
// Relative paths resolve against the launcher's working directory, as for sandboxed tools.
type ExecSpec struct {
	ReadOnly  []string `json:"ro"`
	ReadWrite []string `json:"rw"`
}

// Command builds the command that runs name with args under spec via the launcher
// executable (normally os.Executable()). The process gets its own process group and is
// killed when its parent dies (Linux), so confined servers never outlive Aura.
func Command(executable, workDir string, spec ExecSpec, name string, args ...string) (*exec.Cmd, error) {
	encoded, err := json.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("encoding sandbox spec: %w", err)
	}

	cmd := exec.Command(executable, append([]string{ExecArg, string(encoded), name}, args...)...)
	cmd.Dir = workDir
	cmd.SysProcAttr = childAttributes()

	return cmd, nil
}

// ExecMain is the launcher: it applies spec (args[0]) with Landlock and replaces itself
// with args[1:]. It never runs the command unconfined: any failure exits with status 126.
func ExecMain(args []string) {
	if len(args) < 2 {
		launchFailure("usage: %s <spec> <command> [args...]", ExecArg)
	}

	var spec ExecSpec

	if err := json.Unmarshal([]byte(args[0]), &spec); err != nil {
		launchFailure("decoding spec: %v", err)
	}

	if !IsAvailable() {
		launchFailure("Landlock is not available on this kernel; refusing to start %q unconfined", args[1])
	}

	// Resolve the executable before confinement narrows what can be searched.
	path, err := exec.LookPath(args[1])
	if err != nil {
		launchFailure("%v", err)
	}

	workDir, err := os.Getwd()
	if err != nil {
		launchFailure("resolving working directory: %v", err)
	}

	sb := &Sandbox{WorkDir: workDir}
	sb.AddReadOnly(spec.ReadOnly...)
	sb.AddReadWrite(spec.ReadWrite...)

	if err := sb.Apply(); err != nil {
		launchFailure("%v", err)
	}

	if err := replaceProcess(path, args[1:], os.Environ()); err != nil {
		launchFailure("executing %s: %v", path, err)
	}
}

// launchFailure reports a launcher failure on stderr and exits without running anything.
func launchFailure(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "aura sandbox: "+format+"\n", args...)
	os.Exit(126)
}
