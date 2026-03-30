package autocomplete

import (
	"strings"

	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// directives is the ordered list of completable directive names (without @).
var directives = []string{"File[", "Bash[", "Image[", "Path["}

// Completer provides directive name and path completions.
type Completer struct {
	workdir string
}

// New creates a Completer rooted at the given working directory.
func New(workdir string) *Completer {
	return &Completer{workdir: workdir}
}

// Complete returns a ghost-text suggestion for the given input text at cursorCol.
// Returns "" if no completion applies.
func (c *Completer) Complete(text string, cursorCol int) string {
	if cursorCol > len(text) {
		cursorCol = len(text)
	}

	before := text[:cursorCol]

	// Find the last @ before cursor
	atIdx := strings.LastIndex(before, "@")
	if atIdx < 0 {
		return ""
	}

	afterAt := before[atIdx+1:]

	// Check if we're inside brackets (path completion mode)
	bracketIdx := strings.Index(afterAt, "[")
	if bracketIdx >= 0 {
		// Already inside brackets — check if bracket is closed
		if strings.Contains(afterAt[bracketIdx:], "]") {
			return "" // Directive is fully closed
		}

		return c.completePath(afterAt, bracketIdx)
	}

	// Directive name completion
	return completeDirectiveName(afterAt)
}

// completeDirectiveName returns the suffix to complete a partial directive name.
// partial is the text after @ (e.g., "Fi" for "@Fi").
func completeDirectiveName(partial string) string {
	for _, d := range directives {
		if len(partial) == 0 {
			return d // Just "@" typed — suggest first directive
		}

		if strings.HasPrefix(strings.ToLower(d), strings.ToLower(partial)) && len(partial) < len(d) {
			return d[len(partial):]
		}
	}

	return ""
}

// completePath returns the suffix to complete a partial path inside brackets.
// afterAt is everything after @ (e.g., "File[inter"), bracketIdx is the position of '[' within afterAt.
func (c *Completer) completePath(afterAt string, bracketIdx int) string {
	directiveName := strings.ToLower(afterAt[:bracketIdx])

	// No path completion for @Bash[] — commands are freeform
	if directiveName == "bash" {
		return ""
	}

	partial := afterAt[bracketIdx+1:]

	return c.suggestPath(partial)
}

// suggestPath returns the completion suffix for a partial path.
func (c *Completer) suggestPath(partial string) string {
	dir, prefix := splitPartialPath(partial, c.workdir)

	entries, err := folder.New(dir).List()
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		name := entry.Name()

		// Skip hidden files unless the user is explicitly typing a dot prefix
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(prefix, ".") {
			continue
		}

		if !strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
			continue
		}

		suffix := name[len(prefix):]

		if entry.IsDir() {
			return suffix + "/"
		}

		return suffix + "]"
	}

	return ""
}

// splitPartialPath splits a partial path into directory and filename prefix.
// Returns the absolute directory to read and the filename prefix to match.
func splitPartialPath(partial, workdir string) (string, string) {
	if partial == "" {
		return workdir, ""
	}

	// If partial ends with /, list that directory
	if strings.HasSuffix(partial, "/") {
		d := folder.New(partial)
		if !d.IsAbs() {
			d = folder.New(workdir, partial)
		}

		return d.Path(), ""
	}

	// Split into dir + filename prefix
	f := file.New(partial)
	dirPart := f.Dir()
	prefix := f.Base()

	absDir := folder.New(dirPart)
	if !absDir.IsAbs() {
		absDir = folder.New(workdir, dirPart)
	}

	return absDir.Path(), prefix
}
