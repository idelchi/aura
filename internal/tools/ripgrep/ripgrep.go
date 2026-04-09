// Package ripgrep provides regex search through files using ripgrep.
package ripgrep

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/godyl/pkg/path/file"
)

const defaultTimeout = 30 * time.Second

// Inputs defines the parameters for the RipGrep tool.
type Inputs struct {
	Pattern string `json:"pattern"        jsonschema:"required,description=Regular expression pattern to search for"           validate:"required"`
	Path    string `json:"path,omitempty" jsonschema:"description=File or directory to search in (default: current directory)"`
}

// Tool implements regex search using ripgrep.
type Tool struct {
	tool.Base
}

// New creates a new Rg tool.
func New() *Tool {
	return &Tool{
		Base: tool.Base{
			Text: tool.Text{
				Description: "Fast regex search through files using ripgrep. Returns matching lines with file paths and line numbers.",
				Usage:       "Search for regex patterns across files. Results are printed as path:line:match. Omit path to search the current directory recursively. For advanced options (globs, file types, context lines) use Bash with rg flags directly.",
				Examples: heredoc.Doc(`
					{"pattern": "TODO", "path": "./src"}
					{"pattern": "func.*Error", "path": "./internal"}
					{"pattern": "import.*context"}
				`),
			},
		},
	}
}

// Name returns the tool's identifier.
func (t *Tool) Name() string {
	return "Rg"
}

// Available checks if rg is installed in PATH.
func (t *Tool) Available() bool {
	return file.New("rg").InPath()
}

// Schema returns the provider-agnostic tool definition.
func (t *Tool) Schema() tool.Schema {
	return tool.GenerateSchema[Inputs](t)
}

// Execute runs ripgrep search.
func (t *Tool) Execute(ctx context.Context, args map[string]any) (string, error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return "", err
	}

	params.Path = tool.ResolvePath(ctx, os.ExpandEnv(params.Path))

	if err := tool.ValidatePath(params.Path); err != nil {
		return "", err
	}

	rg, err := file.New("rg").Which()
	if err != nil {
		return "", fmt.Errorf("rg not found in PATH: %w", err)
	}

	rgPath := rg.Path()

	// Build command arguments - keep it simple
	cmdArgs := []string{
		"-n", // Line numbers
		params.Pattern,
	}

	// Add path if specified
	if params.Path != "" {
		cmdArgs = append(cmdArgs, params.Path)
	}

	// Execute with timeout
	execCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, rgPath, cmdArgs...)

	// Set working directory so rg resolves relative paths correctly.
	if wd := tool.WorkDirFromContext(ctx); wd != "" {
		cmd.Dir = wd
	}

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	output := stdout.String()

	// Handle errors
	if err != nil {
		// Exit code 1 means no matches (not an error)
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) {
			return "", nil
		}

		// Timeout
		if execCtx.Err() == context.DeadlineExceeded {
			return output, fmt.Errorf("search timed out after %v", defaultTimeout)
		}

		// Real error
		if stderr.Len() > 0 {
			return "", fmt.Errorf("rg error: %s", stderr.String())
		}

		return "", fmt.Errorf("rg: %w", err)
	}

	return output, nil
}
