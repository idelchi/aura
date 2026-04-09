// Package glob provides file pattern matching using doublestar glob.
package glob

import (
	"context"
	"fmt"
	"os"
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
		Base: tool.Base{
			Text: tool.Text{
				Description: `Find files by filename/path pattern matching (not content search). Uses glob syntax with ** recursive support.`,
				Usage: heredoc.Doc(`
					Matches against file PATHS, not file contents. Not a search/grep tool.
					Use ** for recursive directory matching, * for filename wildcards, ? for single char.
					Pattern must be a valid glob, not a plain substring.

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

	// Sort for consistent output
	slices.Sort(matches)

	return fmt.Sprintf("Found %d match(es):\n%s", len(matches), strings.Join(matches, "\n")), nil
}
