package gitutil

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// DeriveName extracts a directory name from a git URL or path.
// Strips trailing .git, takes the last path segment, and removes the given prefix.
func DeriveName(source, prefix string) string {
	name := source

	name = strings.TrimSuffix(name, ".git")

	// Handle SSH URLs like git@host:org/repo
	if strings.Contains(name, ":") && !strings.Contains(name, "://") {
		parts := strings.SplitN(name, ":", 2)
		if len(parts) == 2 {
			name = parts[1]
		}
	}

	name = file.New(name).Base()
	name = strings.TrimPrefix(name, prefix)

	return name
}

// IsGitURL returns true if the source looks like a git URL (HTTP/SSH).
func IsGitURL(source string) bool {
	return isSSHURL(source) ||
		strings.HasPrefix(source, "http://") ||
		strings.HasPrefix(source, "https://")
}

// CopyDir recursively copies a directory tree from src to dst.
func CopyDir(src, dst string) error {
	srcFolder := folder.New(src)
	if !srcFolder.Exists() {
		return fmt.Errorf("source directory %s does not exist", src)
	}

	return srcFolder.Walk(func(p file.File, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := p.RelativeTo(src)
		if err != nil {
			return err
		}

		target := file.New(dst, rel.Path())

		if d.IsDir() {
			return folder.New(target.Path()).Create()
		}

		return p.Copy(target)
	})
}
