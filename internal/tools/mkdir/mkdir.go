// Package mkdir provides directory creation operations.
package mkdir

import (
	"context"
	"fmt"
	"os"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// Inputs defines the parameters for the Mkdir tool.
type Inputs struct {
	Paths []string `json:"paths" jsonschema:"required,description=Directory paths to create" validate:"required,min=1,dive,required"`
}

// Tool implements directory creation.
type Tool struct {
	tool.Base
}

// New creates a new Mkdir tool with documentation.
func New() *Tool {
	return &Tool{
		Base: tool.Base{
			Text: tool.Text{
				Description: `Create directories, including parent directories if needed`,
				Usage:       `Provide one or more directory paths to create. Parent directories are created automatically.`,
				Examples: heredoc.Doc(`
					{"paths": ["/tmp/newdir"]}
					{"paths": ["./build/output", "./build/cache"]}
					{"paths": ["/home/user/projects/new/deeply/nested"]}
				`),
			},
		},
	}
}

// Name returns the tool's identifier.
func (t *Tool) Name() string {
	return "Mkdir"
}

// Schema returns the provider-agnostic tool definition.
func (t *Tool) Schema() tool.Schema {
	return tool.GenerateSchema[Inputs](t)
}

// Execute creates the specified directories.
func (t *Tool) Execute(ctx context.Context, args map[string]any) (string, error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return "", err
	}

	var created, skipped int

	for _, path := range params.Paths {
		path = tool.ResolvePath(ctx, os.ExpandEnv(path))

		if err := tool.ValidatePath(path); err != nil {
			return "", err
		}

		// Check if already exists
		f := file.New(path)
		if f.Exists() {
			if !f.IsDir() {
				return "", fmt.Errorf("path exists but is not a directory: %s", path)
			}

			skipped++

			continue
		}

		// Create directory with parents
		if err := folder.New(path).Create(); err != nil {
			return "", tool.FileError("creating directory", path, err)
		}

		created++
	}

	if created == 0 && skipped > 0 {
		return fmt.Sprintf("All %d directories already exist", skipped), nil
	}

	if skipped > 0 {
		return fmt.Sprintf("Created %d directories, %d already existed", created, skipped), nil
	}

	return fmt.Sprintf("Created %d directories", created), nil
}

// Paths returns the filesystem paths this tool call will access.
func (t *Tool) Paths(ctx context.Context, args map[string]any) (read, write []string, err error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return nil, nil, err
	}

	write = make([]string, len(params.Paths))
	for i, p := range params.Paths {
		write[i] = tool.ResolvePath(ctx, os.ExpandEnv(p))
	}

	return nil, write, nil
}
