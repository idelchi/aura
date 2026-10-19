package patch

import (
	"context"
	"os"
	"strings"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/tools/filetime"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/godyl/pkg/path/file"
)

// Inputs defines the parameters for the Patch tool.
type Inputs struct {
	Patch string `json:"patch" jsonschema:"required,description=Patch content in context-aware diff format (see Usage)" validate:"required"`
}

// Tool implements the Patch tool for applying file patches.
type Tool struct {
	tool.Base
}

// New creates a new Patch tool.
func New() *Tool {
	return &Tool{
		Text: tool.Text{
			Description: heredoc.Doc(`
					Applies patches to files using a context-aware diff format.

					Supports three operations:
					- Add File: Create a new file or overwrite an existing one with content
					- Update File: Modify existing file content using fuzzy context matching
					- Delete File: Remove an existing file

					The patch format uses context markers (@@) to locate changes.
				`),
			Usage: heredoc.Doc(`
					Format:
					*** Begin Patch
					*** Add File: path/to/new.go
					+line 1
					+line 2
					*** Update File: path/to/existing.go
					@@ function_name
					-old line
					+new line
					*** Delete File: path/to/obsolete.go
					*** End Patch

					Lines prefixed with + are added, - are removed, space is context for matching.
					To rename a file, put *** Move to: new/path immediately after its Update File header.
				`),
			Examples: heredoc.Doc(`
					{"patch": "*** Begin Patch\n*** Add File: hello.txt\n+Hello World\n*** End Patch"}
					{"patch": "*** Begin Patch\n*** Update File: main.go\n@@ func main\n-    return nil\n+    return 42\n*** End Patch"}
					{"patch": "*** Begin Patch\n*** Delete File: obsolete.txt\n*** End Patch"}
				`),
		},
	}
}

// Name returns the tool's identifier.
func (t *Tool) Name() string {
	return "Patch"
}

// Schema returns the provider-agnostic tool definition.
func (t *Tool) Schema() tool.Schema {
	return tool.GenerateSchema[Inputs](t)
}

// parseAndValidate validates the input and every affected path.
// Content preflight happens against the staged filesystem in Preview and Execute.
func (t *Tool) parseAndValidate(ctx context.Context, args map[string]any) ([]Hunk, error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return nil, err
	}

	hunks, err := Parse(params.Patch)
	if err != nil {
		return nil, err
	}

	for _, hunk := range hunks {
		for _, p := range hunk.WritePaths() {
			resolved := tool.ResolvePath(ctx, os.ExpandEnv(p))
			if err := tool.ValidatePath(resolved); err != nil {
				return nil, err
			}
		}
	}

	return hunks, nil
}

// Preview generates a unified diff preview for confirmation dialogs.
func (t *Tool) Preview(ctx context.Context, args map[string]any) (string, error) {
	hunks, err := t.parseAndValidate(ctx, args)
	if err != nil {
		return "", err
	}

	prepared, err := prepare(ctx, hunks)
	if err != nil {
		return "", err
	}
	var parts []string
	for _, c := range prepared.changes {
		d := c.Preview()
		if d != "" {
			parts = append(parts, d)
		}
	}

	return strings.Join(parts, "\n"), nil
}

// Execute parses, validates, and applies hunks.
func (t *Tool) Execute(ctx context.Context, args map[string]any) (string, error) {
	hunks, err := t.parseAndValidate(ctx, args)
	if err != nil {
		return "", err
	}

	prepared, err := prepare(ctx, hunks)
	if err != nil {
		return "", err
	}
	return prepared.Apply(ctx)
}

// Pre parses the patch, validates hunks, and enforces filetime per policy.
// Delete hunks check policy.Delete; all other hunks check policy.Write.
func (t *Tool) Pre(ctx context.Context, args map[string]any) error {
	hunks, err := t.parseAndValidate(ctx, args)
	if err != nil {
		return err
	}

	tracker := filetime.FromContext(ctx)
	policy := tracker.Policy()

	for _, hunk := range hunks {
		_, isDelete := hunk.(*DeleteFile)

		for _, p := range hunk.WritePaths() {
			resolved := tool.ResolvePath(ctx, p)

			if !file.New(resolved).Exists() {
				continue
			}

			if isDelete && !policy.Delete {
				continue
			}

			if !isDelete && !policy.Write {
				continue
			}

			if err := tracker.AssertRead(resolved); err != nil {
				return err
			}
		}
	}

	return nil
}

// Post updates filetime state after a successful patch.
func (t *Tool) Post(ctx context.Context, args map[string]any) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		debug.Log("[patch] post-hook validate: %v", err)

		return
	}

	hunks, err := Parse(params.Patch)
	if err != nil {
		debug.Log("[patch] post-hook parse: %v", err)

		return
	}

	for _, hunk := range hunks {
		hunk.Track(ctx)
	}
}

// WantsLSP indicates that Patch produces source files that benefit from LSP diagnostics.
func (t *Tool) WantsLSP() bool { return true }

// Paths returns the filesystem paths this tool call will access.
func (t *Tool) Paths(ctx context.Context, args map[string]any) (read, write []string, err error) {
	hunks, err := t.parseAndValidate(ctx, args)
	if err != nil {
		return nil, nil, err
	}

	for _, hunk := range hunks {
		for _, p := range hunk.WritePaths() {
			write = append(write, tool.ResolvePath(ctx, p))
		}
	}

	return nil, write, nil
}
