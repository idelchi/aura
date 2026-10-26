package core

import (
	"embed"
	"errors"

	"github.com/idelchi/godyl/pkg/path/folder"
)

// LaunchDir holds the CWD at process start, before os.Chdir(--workdir).
// Set in PersistentPreRunE, used by tasks for template variable expansion.
var LaunchDir string

// WorkDir holds the effective working directory after os.Chdir(--workdir).
// Defaults to LaunchDir when --workdir is not set.
var WorkDir string

// EmbeddedConfig holds the embedded .aura configuration.
var EmbeddedConfig embed.FS

// ProjectConfigDir returns .aura in the current directory if it exists, or "".
func ProjectConfigDir() string {
	d := folder.New(".aura")
	if d.Exists() {
		return d.Path()
	}

	return ""
}

// ErrReported marks an error that a command already reported in its own format
// (e.g. a JSON envelope on stderr). The entry point exits non-zero without printing it.
var ErrReported = errors.New("reported")

// ErrMCPUnavailable means a required MCP server could not be connected.
var ErrMCPUnavailable = errors.New("required MCP server unavailable")

// ErrConfig means the configuration could not produce an assistant.
var ErrConfig = errors.New("invalid configuration")

// ErrResume means a stored session could not be restored.
var ErrResume = errors.New("restoring session")
