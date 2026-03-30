package core

import (
	"embed"

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
