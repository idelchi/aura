// Package read provides the MemoryRead tool for reading persistent memory entries.
package read

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/tools/memory"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// Inputs defines the parameters for the MemoryRead tool.
type Inputs struct {
	Key   string `json:"key,omitempty"   jsonschema:"description=Specific memory entry to read. Omit to list all entries in scope"`
	Scope string `json:"scope,omitempty" jsonschema:"enum=local,enum=global,description=Storage scope: local (project .aura/memory/) or global (~/.aura/memory/). Defaults to local"`
	Query string `json:"query,omitempty" jsonschema:"description=Keyword search across all memory entries in scope"`
}

// Tool reads memory entries from disk.
type Tool struct {
	tool.Base

	localDir  string
	globalDir string
}

// New creates a MemoryRead tool rooted at the given config and global home directories.
func New(configDir, globalHome string) *Tool {
	return &Tool{
		Base: tool.Base{
			Text: tool.Text{
				Description: "Read persistent memory entries from disk. Retrieve specific entries by key, list all entries, or search by keyword.",
				Usage:       "Read a memory entry by key, list all keys (omit key), or search across entries with query. Use local scope (default) for project notes, global for cross-project.",
				Examples: heredoc.Doc(`
					{"key": "architecture", "scope": "local"}
					{"scope": "local"}
					{"query": "database", "scope": "local"}
				`),
			},
		},
		localDir:  folder.New(configDir, "memory").Path(),
		globalDir: memory.GlobalMemoryDir(globalHome),
	}
}

// Name returns the tool's identifier.
func (t *Tool) Name() string {
	return "MemoryRead"
}

// Schema returns the tool definition.
func (t *Tool) Schema() tool.Schema {
	return tool.GenerateSchema[Inputs](t)
}

// Sandboxable returns false as memory tools manage controlled paths in the filesystem.
func (t *Tool) Sandboxable() bool {
	return false
}

// Execute reads memory entries. Three modes:
//   - key set: read that specific file
//   - query set: search across all files for keyword matches
//   - neither: list all files with their first line as summary
func (t *Tool) Execute(_ context.Context, args map[string]any) (string, error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return "", err
	}

	dir := memory.ResolveDir(params.Scope, t.localDir, t.globalDir)
	if dir == "" {
		return "", errors.New("global memory disabled (no --home configured)")
	}

	switch {
	case params.Key != "":
		return readEntry(dir, params.Key)
	case params.Query != "":
		return searchEntries(dir, params.Query)
	default:
		return listEntries(dir, memory.ResolveScope(params.Scope))
	}
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

	if params.Key != "" {
		return []string{folder.New(dir).WithFile(params.Key + ".md").Path()}, nil, nil
	}

	// List/search reads the whole directory
	return []string{dir}, nil, nil
}

// readEntry reads a single memory file by key.
func readEntry(dir, key string) (string, error) {
	if err := memory.ValidateKey(key); err != nil {
		return "", err
	}

	path := folder.New(dir).WithFile(key + ".md").Path()

	data, err := file.New(path).Read()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("memory entry %q not found", key)
		}

		return "", tool.FileError("reading", path, err)
	}

	return string(data), nil
}

// listEntries lists all memory files with their first line as summary.
func listEntries(dir, scope string) (string, error) {
	memDir := folder.New(dir)
	if !memDir.Exists() {
		return fmt.Sprintf("No memory entries (%s)", scope), nil
	}

	entries, err := memDir.ListFiles()
	if err != nil {
		return "", tool.FileError("listing", dir, err)
	}

	var lines []string

	for _, e := range entries {
		if e.Extension() != "md" {
			continue
		}

		key := strings.TrimSuffix(e.Base(), ".md")
		summary := firstLine(e.Path())

		lines = append(lines, fmt.Sprintf("- %s: %s", key, summary))
	}

	if len(lines) == 0 {
		return fmt.Sprintf("No memory entries (%s)", scope), nil
	}

	return fmt.Sprintf("Memory entries (%s):\n%s", scope, strings.Join(lines, "\n")), nil
}

// searchEntries searches all memory files for a keyword match (case-insensitive).
func searchEntries(dir, query string) (string, error) {
	memDir := folder.New(dir)
	if !memDir.Exists() {
		return "No memory entries to search", nil
	}

	entries, err := memDir.ListFiles()
	if err != nil {
		return "", tool.FileError("searching", dir, err)
	}

	queryLower := strings.ToLower(query)

	var results []string

	for _, e := range entries {
		if e.Extension() != "md" {
			continue
		}

		key := strings.TrimSuffix(e.Base(), ".md")

		data, err := e.Read()
		if err != nil {
			debug.Log("[memory] search: skipping %s: %v", e.Base(), err)

			continue
		}

		content := string(data)
		if !strings.Contains(strings.ToLower(content), queryLower) {
			continue
		}

		// Find matching lines for context
		var matchLines []string

		scanner := bufio.NewScanner(strings.NewReader(content))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(strings.ToLower(line), queryLower) {
				matchLines = append(matchLines, "  "+strings.TrimSpace(line))
			}
		}

		results = append(results, fmt.Sprintf("- %s:\n%s", key, strings.Join(matchLines, "\n")))
	}

	if len(results) == 0 {
		return fmt.Sprintf("No matches for %q", query), nil
	}

	return fmt.Sprintf("Search results for %q:\n%s", query, strings.Join(results, "\n")), nil
}

// firstLine reads the first non-empty line of a file for use as a summary.
func firstLine(path string) string {
	f, err := file.New(path).Open()
	if err != nil {
		return "(unreadable)"
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			if len(line) > 80 {
				return line[:80] + "..."
			}

			return line
		}
	}

	return "(empty)"
}
