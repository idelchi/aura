// Package write provides full-file write operations.
package write

import (
	"context"
	"fmt"
	"os"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/diffpreview"
	"github.com/idelchi/aura/internal/tools/filetime"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// Inputs defines the parameters for the Write tool.
type Inputs struct {
	Path    string `json:"path"    jsonschema:"required,description=File path to write (absolute or relative)" validate:"required"`
	Content string `json:"content" jsonschema:"required,description=Complete file content to write"            validate:"required"`
}

// Tool implements full-file writing.
type Tool struct {
	tool.Base
}

// New creates a new Write tool.
func New() *Tool {
	return &Tool{
		Base: tool.Base{
			Text: tool.Text{
				Description: heredoc.Doc(`
					Write complete content to a file. Creates parent directories if needed.

					Use this for new files or full rewrites. For targeted string replacements,
					use Edit. For multi-hunk changes with context markers, use Patch.

					If the file already exists, you must Read it first.
				`),
				Usage: heredoc.Doc(`
					Provide a file path and the complete file content.

					For new files, just provide path and content.
					For existing files, Read the file first, then Write with the full replacement content.
				`),
				Examples: heredoc.Doc(`
					{"path": "cmd/main.go", "content": "package main\n\nfunc main() {\n}\n"}
					{"path": "config/settings.yaml", "content": "debug: true\nport: 8080\n"}
				`),
			},
		},
	}
}

// Name returns the tool's identifier.
func (t *Tool) Name() string {
	return "Write"
}

// Schema returns the provider-agnostic tool definition.
func (t *Tool) Schema() tool.Schema {
	return tool.GenerateSchema[Inputs](t)
}

// Pre enforces read-before-write on existing files when policy.Write is true.
func (t *Tool) Pre(ctx context.Context, args map[string]any) error {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return err
	}

	path := tool.ResolvePath(ctx, os.ExpandEnv(params.Path))

	tracker := filetime.FromContext(ctx)

	policy := tracker.Policy()
	if file.New(path).Exists() && policy.Write {
		return tracker.AssertRead(path)
	}

	return nil
}

// Preview generates a unified diff preview for confirmation dialogs.
func (t *Tool) Preview(ctx context.Context, args map[string]any) (string, error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return "", err
	}

	f := file.New(tool.ResolvePath(ctx, os.ExpandEnv(params.Path)))

	if f.Exists() {
		content, err := f.Read()
		if err != nil {
			return "", err
		}

		return diffpreview.Generate(f.Path(), string(content), params.Content), nil
	}

	return diffpreview.ForNewFile(f.Path(), params.Content), nil
}

// Execute writes content to the specified file path.
func (t *Tool) Execute(ctx context.Context, args map[string]any) (string, error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return "", err
	}

	params.Path = tool.ResolvePath(ctx, os.ExpandEnv(params.Path))

	if err := tool.ValidatePath(params.Path); err != nil {
		return "", err
	}

	f := file.New(params.Path)

	// Create parent directories.
	if dir := f.Dir(); dir != "." {
		if err := folder.New(dir).Create(); err != nil {
			return "", tool.FileError("creating directory", dir, err)
		}
	}

	// Write the file.
	if err := f.Write([]byte(params.Content)); err != nil {
		return "", tool.FileError("writing", f.Path(), err)
	}

	return fmt.Sprintf("W %s (%d bytes)", f, len(params.Content)), nil
}

// Post records that the file content is known (the LLM just provided it).
func (t *Tool) Post(ctx context.Context, args map[string]any) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		debug.Log("[write] post-hook validate: %v", err)

		return
	}

	filetime.FromContext(ctx).RecordRead(tool.ResolvePath(ctx, os.ExpandEnv(params.Path)))
}

// WantsLSP indicates that Write produces source files that benefit from LSP diagnostics.
func (t *Tool) WantsLSP() bool { return true }

// Paths returns the filesystem paths this tool call will access.
func (t *Tool) Paths(ctx context.Context, args map[string]any) (read, write []string, err error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return nil, nil, err
	}

	return nil, []string{tool.ResolvePath(ctx, os.ExpandEnv(params.Path))}, nil
}
