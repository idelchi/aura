//go:build !windows

// Package atomicfile publishes complete private files using the platform's replacement operation.
package atomicfile

import (
	"os"
	"path/filepath"

	"github.com/google/renameio/v2"
)

// WriteFile writes and syncs a same-directory temporary file before replacing path.
// On Unix the file has mode 0600 (subject to umask), and the containing directory is synced.
func WriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := renameio.WriteFile(path, data, 0o600, renameio.WithTempDir(dir)); err != nil {
		return err
	}

	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()

	return directory.Sync()
}
