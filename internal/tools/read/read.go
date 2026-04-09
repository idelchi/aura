// Package read provides file reading operations with line enumeration and range support.
package read

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/lsp"
	"github.com/idelchi/aura/internal/tools/filetime"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/godyl/pkg/path/file"
)

// Inputs defines the parameters for the Read tool.
type Inputs struct {
	// Path is the file path to read.
	Path string `json:"path" jsonschema:"required,description=Path to the file to read" validate:"required"`
	// LineStart is the starting line number (1-based index).
	LineStart int `json:"line_start,omitempty" jsonschema:"description=Starting line number (1-based index)" validate:"omitempty,min=1"`
	// LineEnd is the ending line number (inclusive).
	LineEnd int `json:"line_end,omitempty" jsonschema:"description=Ending line number (inclusive)" validate:"omitempty,min=1,gtefield=LineStart"`
	// Count indicates whether to return only the total line count.
	Count bool `json:"count,omitempty" jsonschema:"description=If true, returns the total number of lines in the file" validate:"omitempty"`
}

// Tool implements file reading with enumerated lines.
type Tool struct {
	tool.Base

	// SmallFileTokens is the token threshold below which line ranges are ignored and the full file is returned.
	// Zero uses the default (2000).
	SmallFileTokens int

	lspManager *lsp.Manager
	estimate   func(string) int
}

// New creates a new Read tool with the specified small file token threshold.
// lspManager is optional (nil disables LSP didOpen notifications).
// estimate is the token estimation function.
func New(smallFileTokens int, lspManager *lsp.Manager, estimate func(string) int) *Tool {
	return &Tool{
		lspManager: lspManager,
		estimate:   estimate,
		Base: tool.Base{
			Text: tool.Text{
				Description: heredoc.Doc(`
					Reads the contents of a file at the specified path.

					Returns the content with rows enumerated as '1: line1', '2: line2', etc.

					Specify a range of lines to read using 'line_start' and 'line_end' parameters. Omit to read the full file.

					Set 'count' to true to get the total number of lines in the file.
				`),
				Usage: heredoc.Doc(`
					Provide a file path to read its contents.

					Use 'count' first to avoid large outputs. If the size is manageable, always read the full file.
					If not, use 'line_start' and 'line_end' to read specific line ranges in chunks.

					ONLY use 'line_start' and 'line_end' for large files to read specific sections.
					For regular files, always read the full content.
				`),
				Examples: heredoc.Doc(`
					{"path": "file.txt"}
					{"path": "file.txt", "line_start": 10, "line_end": 20}
					{"path": "file.txt", "count": true}
				`),
			},
		},
		SmallFileTokens: smallFileTokens,
	}
}

// Name returns the tool's identifier.
func (t *Tool) Name() string {
	return "Read"
}

// Schema returns the provider-agnostic tool definition.
func (t *Tool) Schema() tool.Schema {
	return tool.GenerateSchema[Inputs](t)
}

// Execute reads a file and returns its contents.
func (t *Tool) Execute(ctx context.Context, args map[string]any) (string, error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return "", err
	}

	params.Path = tool.ResolvePath(ctx, os.ExpandEnv(params.Path))

	if err := tool.ValidatePath(params.Path); err != nil {
		return "", err
	}

	data, err := file.New(params.Path).Read()
	if err != nil {
		return "", tool.FileError("reading", params.Path, err)
	}

	if t.lspManager != nil {
		t.lspManager.DidOpen(ctx, params.Path)
	}

	lines := strings.Split(string(data), "\n")

	if params.Count {
		if len(lines) == 1 && lines[0] == "" {
			return "0", nil
		}

		return strconv.Itoa(len(lines)), nil
	}

	from := max(params.LineStart, 1)
	to := params.LineEnd

	if from != 1 && to != 0 {
		threshold := t.SmallFileTokens
		if threshold == 0 {
			threshold = 2000
		}

		if t.estimate(string(data)) < threshold {
			from = 1
			to = 0
		}
	}

	var buf strings.Builder

	for i := 1; i <= len(lines); i++ {
		if i < from {
			continue
		}

		if to > 0 && i > to {
			break
		}

		fmt.Fprintf(&buf, "%d: %s\n", i, lines[i-1])
	}

	return buf.String(), nil
}

// Post records that the file was read for edit-before-read enforcement.
func (t *Tool) Post(ctx context.Context, args map[string]any) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		debug.Log("[read] post-hook validate: %v", err)

		return
	}

	filetime.FromContext(ctx).RecordRead(tool.ResolvePath(ctx, os.ExpandEnv(params.Path)))
}

// Paths returns the filesystem paths this tool call will access.
func (t *Tool) Paths(ctx context.Context, args map[string]any) (read, write []string, err error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return nil, nil, err
	}

	return []string{tool.ResolvePath(ctx, os.ExpandEnv(params.Path))}, nil, nil
}
