// Package edit provides targeted string replacement in files.
package edit

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
)

// Inputs defines the parameters for the Edit tool.
type Inputs struct {
	FilePath   string  `json:"file_path"   jsonschema:"required,description=The absolute path to the file to modify"                               validate:"required"`
	OldString  string  `json:"old_string"  jsonschema:"required,description=The text to replace"                                                   validate:"required"`
	NewString  *string `json:"new_string"  jsonschema:"required,description=The replacement text; use an empty string to delete the matched text" validate:"required"`
	ReplaceAll bool    `json:"replace_all" jsonschema:"description=Replace all occurrences of old_string (default false)"`
}

// Replacement describes one match against the original file.
type Replacement struct {
	OldString  string  `json:"old_string" jsonschema:"required,description=The original text to replace" validate:"required"`
	NewString  *string `json:"new_string" jsonschema:"required,description=The replacement text; empty deletes the match" validate:"required"`
	ReplaceAll bool    `json:"replace_all,omitempty" jsonschema:"description=Replace all occurrences (default false)"`
}

// MultipleInputs groups replacements in one file. All matches must validate before writing.
type MultipleInputs struct {
	FilePath string        `json:"file_path" jsonschema:"required,description=The absolute path to the file to modify" validate:"required"`
	Edits    []Replacement `json:"edits" jsonschema:"required,description=One or more non-overlapping replacements against the original file" validate:"required,min=1,dive"`
}

// Tool implements targeted string replacement.
type Tool struct {
	tool.Base
	multiple bool
}

// New creates a new Edit tool.
func New(multiple bool) *Tool {
	t := &Tool{
		multiple: multiple,
		Text: tool.Text{
			Description: heredoc.Doc(`
					Replaces text in files, trying exact matches first, then line-ending and trailing-whitespace normalization.

					Use this for targeted edits to existing files — changing a variable name,
					fixing a line, updating a value. This tool requires an existing file.

					The file must have been Read first. The edit fails if old_string is not found,
					or if multiple matches exist without replace_all. Leading indentation must match.
				`),
			Usage: heredoc.Doc(`
					Provide file_path, old_string (exact text to find), and new_string (replacement).
					Set replace_all to true to replace every occurrence. Use an empty new_string to delete the match.

					Include enough surrounding context in old_string to ensure a unique match.
				`),
			Examples: heredoc.Doc(`
					{"file_path": "/home/user/main.go", "old_string": "func main() {}", "new_string": "func main() {\n\tfmt.Println(\"hello\")\n}"}
					{"file_path": "/home/user/config.yaml", "old_string": "port: 8080", "new_string": "port: 9090"}
					{"file_path": "/home/user/main.go", "old_string": "oldName", "new_string": "newName", "replace_all": true}
				`),
		},
	}
	if multiple {
		t.Text.Usage = heredoc.Doc(`
			Provide file_path and a non-empty edits array of {old_string, new_string, replace_all?}.
			Use one entry for a simple edit; group related changes in the same file.
			All old_string values match the original file, not earlier replacements.
			Missing, ambiguous, or overlapping matches reject the whole group without writing.
			Include enough context for a unique match; set replace_all to replace every occurrence.
			Use an empty new_string to delete. Use a later call for changes that depend on this call.
		`)
		t.Text.Examples = heredoc.Doc(`
			{"file_path": "/home/user/config.yaml", "edits": [{"old_string": "port: 8080", "new_string": "port: 9090"}]}
			{"file_path": "/home/user/main.go", "edits": [{"old_string": "oldName", "new_string": "newName", "replace_all": true}, {"old_string": "// obsolete\n", "new_string": ""}]}
		`)
	}
	return t
}

// Name returns the tool's identifier.
func (t *Tool) Name() string { return "Edit" }

// IsMultiple reports whether this instance exposes the grouped argument schema.
func (t *Tool) IsMultiple() bool { return t.multiple }

// Parallel prevents concurrent read-modify-write calls from losing file changes.
func (t *Tool) Parallel() bool { return false }

// Schema returns the provider-agnostic tool definition.
func (t *Tool) Schema() tool.Schema {
	if t.multiple {
		return tool.GenerateSchema[MultipleInputs](t)
	}
	return tool.GenerateSchema[Inputs](t)
}

// inputs selects one explicit schema; the model never has to choose between two argument forms.
func (t *Tool) inputs(args map[string]any) (*MultipleInputs, error) {
	if t.multiple {
		return tool.ValidateInput[MultipleInputs](args, t.Schema())
	}
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return nil, err
	}
	return &MultipleInputs{
		FilePath: params.FilePath,
		Edits:    []Replacement{{OldString: params.OldString, NewString: params.NewString, ReplaceAll: params.ReplaceAll}},
	}, nil
}

// Pre enforces read-before-edit policy.
func (t *Tool) Pre(ctx context.Context, args map[string]any) error {
	params, err := t.inputs(args)
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
	params, err := t.inputs(args)
	if err != nil {
		return "", err
	}

	path := tool.ResolvePath(ctx, os.ExpandEnv(params.FilePath))

	content, err := file.New(path).Read()
	if err != nil {
		return "", err
	}

	original := string(content)

	modified, _, err := replaceMany(original, params.Edits)
	if err != nil {
		return "", err
	}

	return diffpreview.Generate(path, original, modified), nil
}

// Execute performs the string replacement.
func (t *Tool) Execute(ctx context.Context, args map[string]any) (string, error) {
	params, err := t.inputs(args)
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

	modified, n, err := replaceMany(original, params.Edits)
	if err != nil {
		return "", err
	}

	if err := f.Write([]byte(modified)); err != nil {
		return "", tool.FileError("writing", path, err)
	}

	return fmt.Sprintf("E %s (%d replacement(s))", f, n), nil
}

// Post records that the file content is known after editing.
func (t *Tool) Post(ctx context.Context, args map[string]any) {
	params, err := t.inputs(args)
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
	params, err := t.inputs(args)
	if err != nil {
		return nil, nil, err
	}

	p := tool.ResolvePath(ctx, os.ExpandEnv(params.FilePath))

	return []string{p}, []string{p}, nil
}
