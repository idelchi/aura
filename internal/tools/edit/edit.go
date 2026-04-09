// Package edit provides targeted string replacement in files.
package edit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/diffpreview"
	"github.com/idelchi/aura/internal/tools/filetime"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/godyl/pkg/path/file"
)

// Inputs defines the parameters for the Edit tool.
type Inputs struct {
	FilePath   string `json:"file_path"   jsonschema:"required,description=The absolute path to the file to modify"                         validate:"required"`
	OldString  string `json:"old_string"  jsonschema:"required,description=The text to replace"                                             validate:"required"`
	NewString  string `json:"new_string"  jsonschema:"required,description=The text to replace it with (must be different from old_string)" validate:"required"`
	ReplaceAll bool   `json:"replace_all" jsonschema:"description=Replace all occurrences of old_string (default false)"`
}

// Tool implements targeted string replacement.
type Tool struct {
	tool.Base
}

// New creates a new Edit tool.
func New() *Tool {
	return &Tool{
		Base: tool.Base{
			Text: tool.Text{
				Description: heredoc.Doc(`
					Performs exact string replacements in files.

					Use this for targeted edits to existing files — changing a variable name,
					fixing a line, updating a value. For new files or complete rewrites, use Write.
					For multi-hunk changes with context markers, use Patch.

					The file must have been Read first. The edit fails if old_string is not found,
					or if multiple matches exist without replace_all.
				`),
				Usage: heredoc.Doc(`
					Provide file_path, old_string (exact text to find), and new_string (replacement).
					Set replace_all to true to replace every occurrence.

					Include enough surrounding context in old_string to ensure a unique match.
				`),
				Examples: heredoc.Doc(`
					{"file_path": "/home/user/main.go", "old_string": "func main() {}", "new_string": "func main() {\n\tfmt.Println(\"hello\")\n}"}
					{"file_path": "/home/user/config.yaml", "old_string": "port: 8080", "new_string": "port: 9090"}
					{"file_path": "/home/user/main.go", "old_string": "oldName", "new_string": "newName", "replace_all": true}
				`),
			},
		},
	}
}

// Name returns the tool's identifier.
func (t *Tool) Name() string { return "Edit" }

// Schema returns the provider-agnostic tool definition.
func (t *Tool) Schema() tool.Schema { return tool.GenerateSchema[Inputs](t) }

// Pre enforces read-before-edit policy.
func (t *Tool) Pre(ctx context.Context, args map[string]any) error {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return err
	}

	path := tool.ResolvePath(ctx, os.ExpandEnv(params.FilePath))

	tracker := filetime.FromContext(ctx)
	if tracker.Policy().Write {
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

	path := tool.ResolvePath(ctx, os.ExpandEnv(params.FilePath))

	content, err := file.New(path).Read()
	if err != nil {
		return "", err
	}

	original := string(content)

	modified, err := replace(original, params.OldString, params.NewString, params.ReplaceAll)
	if err != nil {
		return "", err
	}

	return diffpreview.Generate(path, original, modified), nil
}

// Execute performs the string replacement.
func (t *Tool) Execute(ctx context.Context, args map[string]any) (string, error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return "", err
	}

	path := tool.ResolvePath(ctx, os.ExpandEnv(params.FilePath))

	if err := tool.ValidatePath(path); err != nil {
		return "", err
	}

	f := file.New(path)

	content, err := f.Read()
	if err != nil {
		return "", tool.FileError("reading", path, err)
	}

	original := string(content)

	modified, err := replace(original, params.OldString, params.NewString, params.ReplaceAll)
	if err != nil {
		return "", err
	}

	if err := f.Write([]byte(modified)); err != nil {
		return "", tool.FileError("writing", path, err)
	}

	n := strings.Count(original, params.OldString)
	if !params.ReplaceAll {
		n = 1
	}

	return fmt.Sprintf("E %s (%d replacement(s))", f, n), nil
}

// Post records that the file content is known after editing.
func (t *Tool) Post(ctx context.Context, args map[string]any) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		debug.Log("[edit] post-hook validate: %v", err)

		return
	}

	filetime.FromContext(ctx).RecordRead(tool.ResolvePath(ctx, os.ExpandEnv(params.FilePath)))
}

// WantsLSP indicates that Edit produces source changes that benefit from LSP diagnostics.
func (t *Tool) WantsLSP() bool { return true }

// Paths returns the filesystem paths this tool call will access.
func (t *Tool) Paths(ctx context.Context, args map[string]any) (read, write []string, err error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return nil, nil, err
	}

	p := tool.ResolvePath(ctx, os.ExpandEnv(params.FilePath))

	return []string{p}, []string{p}, nil
}

// replace performs the string replacement with validation.
func replace(content, old, new_ string, all bool) (string, error) {
	if old == new_ {
		return "", errors.New("old_string and new_string are identical")
	}

	count := strings.Count(content, old)
	if count == 0 {
		return "", errors.New("old_string not found in file")
	}

	if count > 1 && !all {
		return "", fmt.Errorf(
			"found %d matches for old_string — use replace_all to replace all, or add more context to match uniquely",
			count,
		)
	}

	if all {
		return strings.ReplaceAll(content, old, new_), nil
	}

	return strings.Replace(content, old, new_, 1), nil
}
