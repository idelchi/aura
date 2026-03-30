package tui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/x/ansi"

	"github.com/idelchi/aura/pkg/llm/tool/call"
)

// highlightReadSyntax applies Chroma syntax highlighting to file content.
// Returns ANSI-highlighted string, or empty string on any error (caller falls back to generic).
func highlightReadSyntax(content, path string) string {
	lexer := lexers.Match(path)
	if lexer == nil {
		return ""
	}

	lexer = chroma.Coalesce(lexer)

	iter, err := lexer.Tokenise(nil, content)
	if err != nil {
		return ""
	}

	formatter := formatters.Get("terminal256")
	style := styles.Get("monokai")

	var buf strings.Builder
	if err := formatter.Format(&buf, style, iter); err != nil {
		return ""
	}

	return buf.String()
}

// highlightDiff applies Chroma syntax highlighting to unified diff content.
// Returns ANSI-highlighted string, or the original content on any error.
func highlightDiff(content string) string {
	lexer := lexers.Get("diff")
	if lexer == nil {
		return content
	}

	lexer = chroma.Coalesce(lexer)

	iter, err := lexer.Tokenise(nil, content)
	if err != nil {
		return content
	}

	formatter := formatters.Get("terminal256")
	style := styles.Get("monokai")

	var buf strings.Builder
	if err := formatter.Format(&buf, style, iter); err != nil {
		return content
	}

	return buf.String()
}

// highlightRgMatches wraps regex pattern matches in magenta ANSI.
// Returns ANSI-highlighted string, or empty string on error (caller falls back to generic).
func highlightRgMatches(content, pattern string) string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return ""
	}

	return re.ReplaceAllStringFunc(content, func(m string) string {
		return "\033[35m" + m + "\033[0m"
	})
}

// renderToolResult produces highlighted output for supported tools (Read, Rg).
// Returns (rendered_string, true) for highlighted tools, ("", false) for generic fallback.
func renderToolResult(tc *call.Call, width int) (string, bool) {
	preview := strings.TrimSpace(tc.Preview)
	if preview == "" {
		return "", false
	}

	var body string

	switch tc.Name {
	case "Read":
		path, ok := tc.Arguments["path"].(string)
		if !ok {
			return "", false
		}

		body = highlightReadSyntax(preview, path)
	case "Rg":
		pattern, ok := tc.Arguments["pattern"].(string)
		if !ok {
			return "", false
		}

		body = highlightRgMatches(preview, pattern)
	default:
		return "", false
	}

	if body == "" {
		return "", false
	}

	// Token header uses standard style; body has raw ANSI — bypass lipgloss.
	header := toolResultStyle.Render(fmt.Sprintf("[tokens: ~%d]", tc.ResultTokens))

	return header + "\n→ " + ansi.Wordwrap(body, width, ""), true
}
