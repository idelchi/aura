package tui

import (
	"strings"

	"github.com/idelchi/godyl/pkg/path/file"
)

// history manages input history with navigation and file persistence.
// It owns entries, navigation state (index, draft), and disk I/O — a single
// type for what was previously split across pkg/history and TUI model fields.
type history struct {
	entries []string  // All entries, oldest first
	index   int       // Navigation position (-1 = not browsing)
	draft   string    // Saved input when user starts browsing
	path    file.File // File path for persistence
	max     int       // Max entries to keep
}

// newHistory creates a history bound to the given file path with a maximum entry count.
func newHistory(path string, max int) *history {
	return &history{
		path:  file.New(path),
		max:   max,
		index: -1,
	}
}

// Load reads entries from the history file. Missing files are silently ignored.
// File format: one entry per line, literal `\n` for embedded newlines (compatible with chzyer/readline).
func (h *history) Load() error {
	if !h.path.Exists() {
		return nil
	}

	data, err := h.path.Read()
	if err != nil {
		return err
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")

	h.entries = make([]string, 0, len(lines))

	for _, line := range lines {
		if line == "" {
			continue
		}

		h.entries = append(h.entries, strings.ReplaceAll(line, `\n`, "\n"))
	}

	h.truncate()

	return nil
}

// Add appends an entry, truncates to max, and immediately persists to disk.
func (h *history) Add(entry string) {
	h.entries = append(h.entries, entry)
	h.truncate()

	// Best-effort persist
	_ = h.save()
}

// Up navigates to an older history entry. On the first call it saves current
// as draft. Returns the text to display and whether navigation occurred.
func (h *history) Up(current string) (string, bool) {
	if len(h.entries) == 0 {
		return "", false
	}

	if h.index == -1 {
		h.draft = current
		h.index = len(h.entries) - 1
	} else if h.index > 0 {
		h.index--
	}

	return h.entries[h.index], true
}

// Down navigates to a newer history entry. When reaching the end it restores
// the saved draft. Returns the text to display and whether navigation occurred.
func (h *history) Down() (string, bool) {
	if h.index == -1 {
		return "", false
	}

	if h.index < len(h.entries)-1 {
		h.index++

		return h.entries[h.index], true
	}

	// Restore draft
	text := h.draft

	h.index = -1
	h.draft = ""

	return text, true
}

// Reset exits browsing mode. Call on Enter (submission).
func (h *history) Reset() {
	h.index = -1
	h.draft = ""
}

// Suggest returns the most recent entry matching the given prefix, or "".
// Skips exact matches (no point suggesting what's already typed).
// Returns the full entry — callers strip the prefix to get the ghost suffix.
func (h *history) Suggest(prefix string) string {
	if prefix == "" || h.index != -1 {
		return ""
	}

	for i := len(h.entries) - 1; i >= 0; i-- {
		entry := h.entries[i]
		if entry != prefix && strings.HasPrefix(entry, prefix) {
			return entry
		}
	}

	return ""
}

// Empty reports whether there are no history entries.
func (h *history) Empty() bool {
	return len(h.entries) == 0
}

// save writes all entries to the history file.
func (h *history) save() error {
	lines := make([]string, len(h.entries))
	for i, entry := range h.entries {
		lines[i] = strings.ReplaceAll(entry, "\n", `\n`)
	}

	return h.path.Write([]byte(strings.Join(lines, "\n") + "\n"))
}

// truncate keeps only the last max entries.
func (h *history) truncate() {
	if len(h.entries) > h.max {
		h.entries = h.entries[len(h.entries)-h.max:]
	}
}
