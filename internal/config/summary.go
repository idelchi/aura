package config

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// SummaryEntry represents a single named config item with its source file and optional detail.
type SummaryEntry struct {
	Name   string
	Source string
	Detail string
}

// SummarySection groups config entries of the same type under a label.
type SummarySection struct {
	Label   string
	Entries []SummaryEntry
}

// Summary holds all the display data for a structured config overview.
type Summary struct {
	Homes      []string
	Sections   []SummarySection
	Features   Features
	LSPServers LSPServers
}

// Display renders the summary as a human-readable string.
func (s Summary) Display() string {
	var b strings.Builder

	// Config dirs header
	b.WriteString("Config dirs:\n")

	if len(s.Homes) == 0 {
		b.WriteString("  (none)\n")
	} else {
		for i, home := range s.Homes {
			if home == "" {
				continue
			}

			label := ""

			if i == len(s.Homes)-1 {
				label = " (write home)"
			}

			fmt.Fprintf(&b, "  %s%s\n", home, label)
		}
	}

	// Sections
	for _, sec := range s.Sections {
		if len(sec.Entries) == 0 {
			continue
		}

		fmt.Fprintf(&b, "\n%s (%d):\n", sec.Label, len(sec.Entries))

		// Calculate column widths for alignment.
		nameWidth := 0
		sourceWidth := 0

		for _, e := range sec.Entries {
			if len(e.Name) > nameWidth {
				nameWidth = len(e.Name)
			}

			if len(e.Source) > sourceWidth {
				sourceWidth = len(e.Source)
			}
		}

		for _, e := range sec.Entries {
			if e.Detail != "" {
				fmt.Fprintf(&b, "  %-*s  %-*s  %s\n", nameWidth, e.Name, sourceWidth, e.Source, e.Detail)
			} else {
				fmt.Fprintf(&b, "  %-*s  %s\n", nameWidth, e.Name, e.Source)
			}
		}
	}

	// Features
	b.WriteString("\nFeatures:\n")
	b.WriteString(s.Features.SummaryDisplay())
	b.WriteString(s.LSPDisplay())

	return b.String()
}

// SummaryDisplay renders a one-line-per-feature summary for the --print output.
func (f Features) SummaryDisplay() string {
	var b strings.Builder

	write := func(label string, parts ...string) {
		filtered := parts[:0]

		for _, p := range parts {
			if p != "" {
				filtered = append(filtered, p)
			}
		}

		if len(filtered) == 0 {
			fmt.Fprintf(&b, "  %-18s (not configured)\n", label+":")
		} else {
			fmt.Fprintf(&b, "  %-18s %s\n", label+":", strings.Join(filtered, ", "))
		}
	}

	// compaction
	write("compaction",
		kv("threshold", fmt.Sprintf("%.0f%%", f.Compaction.Threshold)),
		kv("agent", f.Compaction.Agent),
		kv("chunks", strconv.Itoa(f.Compaction.Chunks)),
	)

	// title
	if f.Title.IsDisabled() {
		write("title", "disabled")
	} else {
		write("title",
			kv("agent", f.Title.Agent),
			kv("max_length", strconv.Itoa(f.Title.MaxLength)),
		)
	}

	// vision
	write("vision", kv("agent", f.Vision.Agent))

	// stt
	write("stt", kv("agent", f.STT.Agent))

	// tts
	write("tts", kv("agent", f.TTS.Agent))

	// embeddings
	write("embeddings", kv("agent", f.Embeddings.Agent))

	// thinking
	write("thinking",
		kv("agent", f.Thinking.Agent),
		kv("prompt", f.Thinking.Prompt),
		kv("keep_last", strconv.Itoa(f.Thinking.KeepLast)),
		kv("token_threshold", strconv.Itoa(f.Thinking.TokenThreshold)),
	)

	// tools
	rbp := f.ToolExecution.ReadBefore.ToPolicy()
	write("tools",
		kv("max_steps", strconv.Itoa(f.ToolExecution.MaxSteps)),
		kv("mode", f.ToolExecution.Mode),
		kv("parallel", formatBoolPtr(f.ToolExecution.Parallel, true)),
		kv("read_before.write", strconv.FormatBool(rbp.Write)),
		kv("read_before.delete", strconv.FormatBool(rbp.Delete)),
	)

	// sandbox
	write("sandbox",
		kv("enabled", strconv.FormatBool(f.Sandbox.IsEnabled())),
	)

	// plugins
	write("plugins",
		kv("unsafe", strconv.FormatBool(f.PluginConfig.Unsafe)),
		kv("include", fmt.Sprintf("%v", f.PluginConfig.Include)),
		kv("exclude", fmt.Sprintf("%v", f.PluginConfig.Exclude)),
	)

	return b.String()
}

// LSPDisplay renders the LSP servers summary line.
func (s Summary) LSPDisplay() string {
	if len(s.LSPServers) == 0 {
		return fmt.Sprintf("  %-18s (not configured)\n", "lsp:")
	}

	serverNames := make([]string, 0, len(s.LSPServers))
	for name, server := range s.LSPServers {
		label := name

		if server.Disabled {
			label += "(disabled)"
		}

		serverNames = append(serverNames, label)
	}

	slices.Sort(serverNames)

	return fmt.Sprintf("  %-18s %s\n", "lsp:", kv("servers", strings.Join(serverNames, " ")))
}

// formatBoolPtr renders a *bool with its default shown when nil.
func formatBoolPtr(p *bool, defaultVal bool) string {
	if p == nil {
		return strconv.FormatBool(defaultVal)
	}

	return strconv.FormatBool(*p)
}

// kv returns "key=value" if value is non-empty, or "" if empty.
func kv(key, value string) string {
	if value == "" {
		return ""
	}

	return key + "=" + value
}

// Summary builds a Summary from the loaded config for display by --print.
func (c Config) Summary(homes []string) Summary {
	s := Summary{
		Homes:      homes,
		Features:   c.Features,
		LSPServers: c.LSPServers,
	}

	// Agents
	s.addSection("Agents", func() []SummaryEntry {
		var entries []SummaryEntry

		for file, agent := range c.Agents {
			detail := ""

			if agent.Metadata.Model.Name != "" {
				detail = kv("model", agent.Metadata.Model.Name)
			}

			if agent.Metadata.Model.Provider != "" {
				if detail != "" {
					detail += " "
				}

				detail += kv("provider", agent.Metadata.Model.Provider)
			}

			entries = append(entries, SummaryEntry{
				Name:   agent.Metadata.Name,
				Source: file.Path(),
				Detail: detail,
			})
		}

		return entries
	})

	// Modes
	s.addSection("Modes", func() []SummaryEntry {
		var entries []SummaryEntry

		for file, mode := range c.Modes {
			entries = append(entries, SummaryEntry{
				Name:   mode.Metadata.Name,
				Source: file.Path(),
			})
		}

		return entries
	})

	// Systems
	s.addSection("Systems", func() []SummaryEntry {
		var entries []SummaryEntry

		for file, system := range c.Systems {
			entries = append(entries, SummaryEntry{
				Name:   system.Metadata.Name,
				Source: file.Path(),
			})
		}

		return entries
	})

	// Providers
	s.addSection("Providers", func() []SummaryEntry {
		var entries []SummaryEntry

		for name, provider := range c.Providers {
			entries = append(entries, SummaryEntry{
				Name:   name,
				Source: provider.Source,
				Detail: kv("type", provider.Type),
			})
		}

		return entries
	})

	// AGENTS.md
	s.addSection("AGENTS.md", func() []SummaryEntry {
		var entries []SummaryEntry

		for file, md := range c.AgentsMd {
			entries = append(entries, SummaryEntry{
				Name:   string(md.Type),
				Source: file.Path(),
			})
		}

		return entries
	})

	// Commands
	s.addSection("Commands", func() []SummaryEntry {
		var entries []SummaryEntry

		for file, cmd := range c.Commands {
			entries = append(entries, SummaryEntry{
				Name:   cmd.Metadata.Name,
				Source: file.Path(),
			})
		}

		return entries
	})

	// Skills
	s.addSection("Skills", func() []SummaryEntry {
		var entries []SummaryEntry

		for file, skill := range c.Skills {
			entries = append(entries, SummaryEntry{
				Name:   skill.Metadata.Name,
				Source: file.Path(),
			})
		}

		return entries
	})

	// MCP Servers
	s.addSection("MCP Servers", func() []SummaryEntry {
		var entries []SummaryEntry

		for name, server := range c.MCPs {
			detail := kv("type", server.Type)
			if !server.IsEnabled() {
				detail = "disabled"
			} else if server.Condition != "" {
				detail += ", " + kv("condition", server.Condition)
			}

			entries = append(entries, SummaryEntry{
				Name:   name,
				Source: server.Source,
				Detail: detail,
			})
		}

		return entries
	})

	// Tool Defs
	s.addSection("Tool Defs", func() []SummaryEntry {
		var entries []SummaryEntry

		for name, def := range c.ToolDefs {
			detail := ""

			if def.Condition != "" {
				detail = kv("condition", def.Condition)
			}

			if def.Parallel != nil {
				if detail != "" {
					detail += " "
				}

				detail += kv("parallel", formatBoolPtr(def.Parallel, true))
			}

			entries = append(entries, SummaryEntry{
				Name:   name,
				Source: def.Source,
				Detail: detail,
			})
		}

		return entries
	})

	// Tasks
	s.addSection("Tasks", func() []SummaryEntry {
		var entries []SummaryEntry

		for name, task := range c.Tasks {
			detail := ""

			if task.Schedule != "" {
				detail = kv("schedule", task.Schedule)
			} else {
				detail = "manual"
			}

			entries = append(entries, SummaryEntry{
				Name:   name,
				Source: task.Source,
				Detail: detail,
			})
		}

		return entries
	})

	// Plugins
	s.addSection("Plugins", func() []SummaryEntry {
		var entries []SummaryEntry

		for name, plugin := range c.Plugins {
			detail := ""

			if !plugin.IsEnabled() {
				detail = "disabled"
			} else if plugin.Condition != "" {
				detail = kv("condition", plugin.Condition)
			}

			entries = append(entries, SummaryEntry{
				Name:   name,
				Source: plugin.Source,
				Detail: detail,
			})
		}

		return entries
	})

	return s
}

// addSection builds a section from the given entry-builder function,
// sorts entries by name, and appends it to the summary.
func (s *Summary) addSection(label string, build func() []SummaryEntry) {
	entries := build()

	slices.SortFunc(entries, func(a, b SummaryEntry) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})

	s.Sections = append(s.Sections, SummarySection{
		Label:   label,
		Entries: entries,
	})
}
