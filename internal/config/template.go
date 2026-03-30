package config

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// ConfigPaths holds config directory paths for template consumption.
// Templates use {{ .Config.Global }}, {{ .Config.Project }}, {{ .Config.Source }}.
type ConfigPaths struct {
	Global  string // ~/.aura (always)
	Project string // write home (last --config entry)
	Source  string // per-file, the config home this agent was loaded from
}

// TemplateData is the context passed to all prompt templates during rendering.
// Templates access fields via Go template syntax, e.g., {{ .Model.Name }}, {{ .Tools.Eager }}.
// Model.Name is always set from agent config. Capability fields (Family, Thinking,
// Vision, etc.) are zero until runtime model resolution via the provider.
type TemplateData struct {
	Config     ConfigPaths
	LaunchDir  string // CWD at process start, before --workdir
	WorkDir    string // CWD after --workdir processing
	Model      ModelData
	Provider   string
	Agent      string
	Mode       ModeData
	Tools      ToolsData        // eager + deferred tool info, filled by BuildAgent
	Vars       map[string]any   // user-defined --set template variables
	Sandbox    SandboxData      // sandbox state, filled by BuildAgent before all template rendering
	ReadBefore ReadBeforeData   // read-before-write policy, filled by BuildAgent
	ToolPolicy ToolPolicyData   // tool policy, filled by BuildAgent
	Hooks      HooksData        // active hook metadata for prompt rendering
	Memories   MemoriesData     // concatenated memory file contents, filled by BuildAgent
	Files      []FileEntry      // autoloaded file entries for composition
	Workspace  []WorkspaceEntry // workspace (AGENTS.md) entries for composition
}

// ModeData holds the active mode metadata for template consumption.
// Templates use {{ .Mode.Name }} to access the mode name.
type ModeData struct {
	Name string
}

// ToolsData holds eager and deferred tool information for template consumption.
// Templates use {{ .Tools.Eager }} and {{ .Tools.Deferred }}.
type ToolsData struct {
	Eager    []string // resolved eager tool names
	Deferred string   // pre-rendered deferred tool index XML (empty = none)
}

// FileEntry represents an autoloaded file registered as a named template.
// Used in composition: {{ range .Files }}{{ include .TemplateName $ }}{{ end }}.
type FileEntry struct {
	Name         string // display name (e.g., "docs/rules.txt")
	TemplateName string // registered template name (e.g., "file:docs/rules.txt")
}

// WorkspaceEntry represents a workspace instruction file registered as a named template.
// Used in composition: {{ range .Workspace }}{{ include .TemplateName $ }}{{ end }}.
type WorkspaceEntry struct {
	Type         string // display header (e.g., "Workspace Instructions: local")
	TemplateName string // registered template name (e.g., "ws:AGENTS.md")
}

// ToAnyMap converts a map[string]string to map[string]any.
// Used at the boundary between CLI parsing (which produces string maps)
// and TemplateData.Vars (which requires map[string]any for sprig compatibility).
func ToAnyMap(m map[string]string) map[string]any {
	if m == nil {
		return nil
	}

	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}

	return out
}

// ModelData holds flattened model metadata for template consumption.
// Capabilities are exposed as booleans for easy conditional use in templates.
type ModelData struct {
	Name            string
	Family          string
	ParameterCount  int64  // raw value for comparisons: {{ if lt .Model.ParameterCount 10000000000 }}
	ParameterSize   string // display string: "8B", "70B", "1.5B"
	ContextLength   int
	Thinking        bool
	ThinkingLevels  bool
	Tools           bool
	Vision          bool
	Embedding       bool
	Reranking       bool
	ContextOverride bool
}

// SandboxData holds sandbox metadata for template consumption.
// Templates can use {{ .Sandbox.Enabled }}, {{ .Sandbox.Requested }}, {{ .Sandbox.Display }}, etc.
type SandboxData struct {
	Enabled   bool // Landlock actually enforcing (available AND requested)
	Requested bool // User wants sandbox (config or runtime toggle, independent of kernel)
	ReadOnly  []string
	ReadWrite []string
	Display   string // Pre-rendered restriction text for prompt injection
}

// NewSandboxData creates template-friendly SandboxData from sandbox state and merged restrictions.
func NewSandboxData(enabled, requested bool, r Restrictions, display string) SandboxData {
	return SandboxData{
		Enabled:   enabled,
		Requested: requested,
		ReadOnly:  r.ReadOnly,
		ReadWrite: r.ReadWrite,
		Display:   display,
	}
}

// ReadBeforeData holds read-before-write policy for template consumption.
// Templates can use {{ .ReadBefore.Write }}, {{ .ReadBefore.Delete }}.
type ReadBeforeData struct {
	Write  bool // must read before writing/patching
	Delete bool // must read before deleting
}

// ToolPolicyData holds tool policy for template consumption.
// Templates can use {{ .ToolPolicy.Display }}, {{ range .ToolPolicy.Auto }}, etc.
type ToolPolicyData struct {
	Auto    []string // auto-approved patterns
	Confirm []string // confirmation-required patterns
	Deny    []string // denied patterns
	Display string   // pre-rendered display text
}

// NewModelData converts a resolved model.Model into template-friendly ModelData.
func NewModelData(m model.Model) ModelData {
	return ModelData{
		Name:            m.Name,
		Family:          m.Family,
		ParameterCount:  int64(m.ParameterCount),
		ParameterSize:   m.ParameterCount.String(),
		ContextLength:   int(m.ContextLength),
		Thinking:        m.Capabilities.Thinking(),
		ThinkingLevels:  m.Capabilities.ThinkingLevels(),
		Tools:           m.Capabilities.Tools(),
		Vision:          m.Capabilities.Vision(),
		Embedding:       m.Capabilities.Embedding(),
		Reranking:       m.Capabilities.Reranking(),
		ContextOverride: m.Capabilities.ContextOverride(),
	}
}

// HookEntry holds metadata for a single active hook, exposed to prompt templates.
// Templates access fields via {{ range .Hooks.Active }}{{ .Name }}{{ end }}.
type HookEntry struct {
	Name        string // hook name from config map key
	Description string // human-readable summary
	Event       string // "pre" or "post"
	Matcher     string // tool name regex (empty = all tools)
	Files       string // file glob pattern (empty = all files)
	Command     string // shell command
}

// HooksData holds active hook metadata for prompt template consumption.
// Templates use {{ .Hooks.Display }} for a pre-rendered summary,
// or {{ range .Hooks.Active }} for fine-grained access to each hook.
type HooksData struct {
	Active  []HookEntry // non-disabled hooks, sorted by event then name
	Display string      // pre-rendered summary text (empty when no hooks)
}

// NewHooksData builds template-friendly HooksData from filtered hooks.
func NewHooksData(hks Hooks) HooksData {
	var entries []HookEntry

	for name, h := range hks {
		if h.IsDisabled() {
			continue
		}

		entries = append(entries, HookEntry{
			Name:        name,
			Description: h.Description,
			Event:       h.Event,
			Matcher:     h.Matcher,
			Files:       h.Files,
			Command:     h.Command,
		})
	}

	slices.SortFunc(entries, func(a, b HookEntry) int {
		if a.Event != b.Event {
			return strings.Compare(a.Event, b.Event)
		}

		return strings.Compare(a.Name, b.Name)
	})

	if len(entries) == 0 {
		return HooksData{}
	}

	var sb strings.Builder

	sb.WriteString("The following hooks run automatically before or after tool calls.\n")
	sb.WriteString("They may modify files after your edits — this is expected behavior, not an error.\n")
	sb.WriteString("Do not revert or investigate changes made by hooks.\n")

	for _, e := range entries {
		sb.WriteString("\n- ")
		sb.WriteString(e.Name)
		sb.WriteString(" (")
		sb.WriteString(e.Event)
		sb.WriteString(")")

		if e.Description != "" {
			sb.WriteString(": ")
			sb.WriteString(e.Description)
		}

		if e.Files != "" {
			sb.WriteString(" [files: ")
			sb.WriteString(e.Files)
			sb.WriteString("]")
		}

		if e.Matcher != "" {
			sb.WriteString(" [tools: ")
			sb.WriteString(e.Matcher)
			sb.WriteString("]")
		}
	}

	return HooksData{
		Active:  entries,
		Display: sb.String(),
	}
}

// MemoriesData holds concatenated memory file contents for template consumption.
// Templates use {{ .Memories.Local }} and {{ .Memories.Global }}.
type MemoriesData struct {
	Local  string // concatenated .aura/memory/*.md contents
	Global string // concatenated ~/.aura/memory/*.md contents
}

// NewMemoriesData reads all *.md files from local and global memory directories
// and concatenates their contents with "---" separators.
// Missing directories produce empty strings. Unreadable files return an error.
func NewMemoriesData(home, globalHome string) (MemoriesData, error) {
	local, err := readMemoryDir(folder.New(home, "memory"))
	if err != nil {
		return MemoriesData{}, fmt.Errorf("local memories: %w", err)
	}

	var global string

	if globalHome != "" {
		global, err = readMemoryDir(folder.New(globalHome, "memory"))
		if err != nil {
			return MemoriesData{}, fmt.Errorf("global memories: %w", err)
		}
	}

	return MemoriesData{Local: local, Global: global}, nil
}

// readMemoryDir reads all *.md files from dir and concatenates them with "---" separators.
// Returns empty string if the directory does not exist.
func readMemoryDir(dir folder.Folder) (string, error) {
	entries, err := dir.ListFiles()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}

		return "", fmt.Errorf("reading directory: %w", err)
	}

	var parts []string

	for _, e := range entries {
		if e.Extension() != "md" {
			continue
		}

		data, err := e.Read()
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", e.Base(), err)
		}

		parts = append(parts, string(data))
	}

	return strings.Join(parts, "\n---\n"), nil
}
