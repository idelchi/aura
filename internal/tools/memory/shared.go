// Package memory provides shared helpers for persistent key-value memory storage tools.
package memory

import (
	"errors"
	"fmt"
	"strings"

	"github.com/idelchi/godyl/pkg/path/folder"
)

// GlobalMemoryDir returns the global memory directory path, or "" if global home is disabled.
func GlobalMemoryDir(globalHome string) string {
	if globalHome == "" {
		return ""
	}

	return folder.New(globalHome, "memory").Path()
}

// ResolveDir returns the directory for the given scope.
func ResolveDir(scope, localDir, globalDir string) string {
	if scope == "global" {
		return globalDir
	}

	return localDir
}

// ResolveScope normalizes an empty scope to "local".
func ResolveScope(scope string) string {
	if scope == "" {
		return "local"
	}

	return scope
}

// ValidateKey checks that a memory key is valid for use as a filename.
func ValidateKey(key string) error {
	if key == "" {
		return errors.New("key cannot be empty")
	}

	if strings.ContainsAny(key, `/\`) || strings.Contains(key, "..") {
		return fmt.Errorf("invalid key %q: must not contain path separators", key)
	}

	return nil
}
