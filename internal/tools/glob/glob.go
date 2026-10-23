// Package glob provides file pattern matching using doublestar glob.
package glob

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/bmatcuk/doublestar/v4"

	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// Inputs defines the parameters for the Glob tool.
type Inputs struct {
	Pattern string `json:"pattern"        jsonschema:"required,description=Glob pattern (supports **, *, ?, [class], {alt1,alt2})" validate:"required"`
	Path    string `json:"path,omitempty" jsonschema:"description=Directory to search in (default: current directory)"`
}

// Tool implements file pattern matching using doublestar glob.
type Tool struct {
	tool.Base
}

// New creates a new Glob tool with documentation.
func New() *Tool {
	return &Tool{
		Text: tool.Text{
			Description: `Find files by filename/path pattern matching (not content search). Uses glob syntax with ** recursive support.`,
			Usage: heredoc.Doc(`
					Matches against file PATHS, not file contents. Not a search/grep tool.
					Use ** for recursive directory matching, * for filename wildcards, ? for single char.
					Pattern must be a valid glob, not a plain substring.
					Results inside the working directory are relative to it, even when path selects a subdirectory.
					Results outside the working directory remain absolute. Pass returned paths directly to Read.

					Examples:
					  **/*.go          - all .go files recursively
					  src/**/test_*.py - test files under src/
					  *.{json,yaml}    - json or yaml in current dir
				`),
			Examples: heredoc.Doc(`
					{"pattern": "**/*.go"}
					{"pattern": "src/**/*_test.py"}
					{"pattern": "*.{json,yaml}"}
					{"pattern": "**/README.md"}
				`),
		},
	}
}

// Name returns the tool's identifier.
func (t *Tool) Name() string {
	return "Glob"
}

// Schema returns the provider-agnostic tool definition.
func (t *Tool) Schema() tool.Schema {
	return tool.GenerateSchema[Inputs](t)
}

// Execute runs the glob pattern matching.
func (t *Tool) Execute(ctx context.Context, args map[string]any) (string, error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return "", err
	}

	params.Path = tool.ResolvePath(ctx, os.ExpandEnv(params.Path))

	if err := tool.ValidatePath(params.Path); err != nil {
		return "", err
	}

	// Build full pattern with path if specified
	pattern := params.Pattern
	if params.Path != "" {
		pattern = folder.New(params.Path).WithFile(params.Pattern).Path()
	}

	// Validate pattern
	if !doublestar.ValidatePathPattern(pattern) {
		return "", fmt.Errorf("invalid pattern: %s", params.Pattern)
	}

	// Execute glob with files-only option
	matches, err := doublestar.FilepathGlob(pattern, doublestar.WithFilesOnly())
	if err != nil {
		return "", fmt.Errorf("glob: %w", err)
	}

	if len(matches) == 0 {
		return "", nil
	}

	// Avoid repeating a potentially long workspace prefix for every local file.
	// External results stay absolute, so each path remains directly usable by Read.
	workingDir, err := filepath.Abs(tool.ResolvePath(ctx, "."))
	if err != nil {
		return "", err
	}
	for i, match := range matches {
		absolute, err := filepath.Abs(match)
		if err != nil {
			return "", err
		}
		matches[i] = absolute
		if relative, err := filepath.Rel(workingDir, absolute); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			matches[i] = relative
		}
	}

	// Sort after formatting for consistent output, including mixed local/external results.
	slices.Sort(matches)

	return fmt.Sprintf("Found %d match(es) (local paths relative to working directory):\n%s", len(matches), strings.Join(matches, "\n")), nil
}
