// Package write provides the MemoryWrite tool for persisting memory entries.
package write

import (
	"context"
	"errors"
	"fmt"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/internal/tools/memory"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// Inputs defines the parameters for the MemoryWrite tool.
type Inputs struct {
	Key     string `json:"key"             jsonschema:"required,description=Filename/topic for the memory entry (e.g. architecture or user-preferences)"`
	Content string `json:"content"         jsonschema:"required,description=Markdown content to persist"`
	Scope   string `json:"scope,omitempty" jsonschema:"enum=local,enum=global,description=Storage scope: local (project .aura/memory/) or global (~/.aura/memory/). Defaults to local"`
}

// Tool persists memory entries to disk as markdown files.
type Tool struct {
	tool.Base

	localDir  string
	globalDir string
}

// New creates a MemoryWrite tool rooted at the given config and global home directories.
func New(configDir, globalHome string) *Tool {
	return &Tool{
		Base: tool.Base{
			Text: tool.Text{
				Description: "Persist a memory entry to disk. Use this to save notes, decisions, patterns, or context that should survive beyond the current conversation or compaction.",
				Usage:       "Write a key-value memory entry. Key becomes the filename, content is markdown. Use local scope (default) for project-specific notes, global for cross-project preferences.",
				Examples: heredoc.Doc(`
					{"key": "architecture", "content": "# Architecture\n\nEvent-driven with message queue.", "scope": "local"}
					{"key": "preferences", "content": "# Preferences\n\n- Always use dark mode\n- Prefer concise output", "scope": "global"}
				`),
			},
		},
		localDir:  folder.New(configDir, "memory").Path(),
		globalDir: memory.GlobalMemoryDir(globalHome),
	}
}

// Name returns the tool's identifier.
func (t *Tool) Name() string {
	return "MemoryWrite"
}

// Schema returns the tool definition.
func (t *Tool) Schema() tool.Schema {
	return tool.GenerateSchema[Inputs](t)
}

// Sandboxable returns false as memory tools manage controlled paths in the filesystem.
func (t *Tool) Sandboxable() bool {
	return false
}

// Execute writes a memory entry to disk.
func (t *Tool) Execute(_ context.Context, args map[string]any) (string, error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return "", err
	}

	if err := memory.ValidateKey(params.Key); err != nil {
		return "", err
	}

	dir := memory.ResolveDir(params.Scope, t.localDir, t.globalDir)
	if dir == "" {
		return "", errors.New("global memory disabled (no --home configured)")
	}

	memDir := folder.New(dir)
	if err := memDir.Create(); err != nil {
		return "", tool.FileError("creating memory directory", dir, err)
	}

	target := memDir.WithFile(params.Key + ".md")

	if err := target.Write([]byte(params.Content)); err != nil {
		return "", tool.FileError("writing", target.Path(), err)
	}

	scope := memory.ResolveScope(params.Scope)

	return fmt.Sprintf("Saved memory: %s (%s)", params.Key, scope), nil
}

// Paths returns the filesystem paths this tool call will access.
func (t *Tool) Paths(_ context.Context, args map[string]any) (read, write []string, err error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return nil, nil, err
	}

	dir := memory.ResolveDir(params.Scope, t.localDir, t.globalDir)
	if dir == "" {
		return nil, nil, errors.New("global memory disabled (no --home configured)")
	}

	return nil, []string{folder.New(dir).WithFile(params.Key + ".md").Path()}, nil
}
