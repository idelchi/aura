// Package ls provides directory listing operations.
package ls

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// Inputs defines the parameters for the Ls tool.
type Inputs struct {
	Path  string `json:"path,omitempty"  jsonschema:"description=Directory path to list (default: current directory)"`
	Depth int    `json:"depth,omitempty" jsonschema:"description=Maximum depth to recurse (1=current dir only, 2=one level deep, etc). Default: 1"`
}

// Tool implements directory listing.
type Tool struct {
	tool.Base
}

// New creates a new Ls tool with documentation.
func New() *Tool {
	return &Tool{
		Base: tool.Base{
			Text: tool.Text{
				Description: `List directory contents with optional recursive depth control`,
				Usage:       `Provide a directory path to list its contents, or omit for current directory. Use depth to recurse into subdirectories`,
				Examples: heredoc.Doc(`
					{}
					{"path": "/home/user/projects"}
					{"path": "./src", "depth": 2}
					{"path": "/tmp", "depth": 3}
				`),
			},
		},
	}
}

// Name returns the tool's identifier.
func (t *Tool) Name() string {
	return "Ls"
}

// Schema returns the provider-agnostic tool definition.
func (t *Tool) Schema() tool.Schema {
	return tool.GenerateSchema[Inputs](t)
}

// Execute lists directory contents.
func (t *Tool) Execute(ctx context.Context, args map[string]any) (string, error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return "", err
	}

	params.Path = os.ExpandEnv(params.Path)

	path := params.Path
	if path == "" {
		path = "."
	}

	depth := params.Depth
	if depth <= 0 {
		depth = 1
	}

	// Resolve relative paths against the effective working directory.
	absPath := tool.ResolvePath(ctx, path)

	if err := tool.ValidatePath(absPath); err != nil {
		return "", err
	}

	f := file.New(absPath)

	if !f.Exists() {
		return "", fmt.Errorf("path not found: %s", absPath)
	}

	// If it's a file, just return the file name
	if !f.IsDir() {
		return f.Base(), nil
	}

	// Collect entries with their relative paths
	type entry struct {
		relPath string
		isDir   bool
	}

	var (
		entries    []entry
		errCount   int
		errSamples []string
	)

	err = folder.New(absPath).Walk(func(fp file.File, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			errCount++

			if len(errSamples) < 3 {
				errSamples = append(errSamples, fmt.Sprintf("%s: %v", fp, walkErr))
			}

			return nil
		}

		// Get path relative to root
		relFile, err := fp.RelativeTo(absPath)
		rel := relFile.Path()

		if err != nil {
			return nil
		}

		// Skip root
		if rel == "." {
			return nil
		}

		// Check depth (count separators + 1)
		currentDepth := strings.Count(rel, string(filepath.Separator)) + 1
		if currentDepth > depth {
			if d.IsDir() {
				return filepath.SkipDir
			}

			return nil
		}

		entries = append(entries, entry{relPath: rel, isDir: d.IsDir()})

		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walking directory: %w", err)
	}

	if len(entries) == 0 {
		return "Empty directory", nil
	}

	// Sort: directories first, then by path
	slices.SortFunc(entries, func(a, b entry) int {
		if a.isDir != b.isDir {
			if a.isDir {
				return -1
			}

			return 1
		}

		return strings.Compare(a.relPath, b.relPath)
	})

	// Format output
	var buf strings.Builder

	for _, e := range entries {
		name := e.relPath
		if e.isDir {
			name += "/"
		}

		buf.WriteString(name)
		buf.WriteString("\n")
	}

	if errCount > 0 {
		fmt.Fprintf(&buf, "\n[%d entries skipped due to errors]\n", errCount)

		for _, s := range errSamples {
			fmt.Fprintf(&buf, "  %s\n", s)
		}
	}

	return strings.TrimSpace(buf.String()), nil
}
