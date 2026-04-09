package glob

import (
	"fmt"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/files"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// Glob returns files under root matching the given glob pattern.
func Glob(root folder.Folder, pattern string) (files.Files, error) {
	criteria := func(file file.File) (bool, error) {
		return doublestar.Match(pattern, file.Path())
	}

	files, err := root.FindFiles(criteria)
	if err != nil {
		return nil, fmt.Errorf("finding files for pattern %q: %w", pattern, err)
	}

	return files, nil
}
