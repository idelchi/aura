package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/idelchi/aura/internal/slash"
	"github.com/idelchi/aura/pkg/clipboard"
	"github.com/idelchi/godyl/pkg/path/file"
)

// Format represents an export output format.
type Format int

const (
	FormatPlaintext Format = iota
	FormatMarkdown
	FormatJSON
	FormatJSONL
)

var formatKeywords = map[string]Format{
	"text":      FormatPlaintext,
	"plaintext": FormatPlaintext,
	"markdown":  FormatMarkdown,
	"md":        FormatMarkdown,
	"json":      FormatJSON,
	"jsonl":     FormatJSONL,
}

var extensionFormats = map[string]Format{
	"txt":   FormatPlaintext,
	"md":    FormatMarkdown,
	"json":  FormatJSON,
	"jsonl": FormatJSONL,
}

// Export creates the /export command to export conversation to clipboard or file.
func Export() slash.Command {
	return slash.Command{
		Name:        "/export",
		Description: "Export conversation to clipboard or file",
		Hints:       "[format] [path|clipboard]",
		Category:    "session",
		Execute: func(_ context.Context, c slash.Context, args ...string) (string, error) {
			format, dest := parseExportArgs(args)
			history := c.Builder().History()

			var (
				content []byte
				err     error
			)

			switch format {
			case FormatMarkdown:
				content = []byte(history.RenderMarkdown())
			case FormatJSON:
				content, err = history.ExportJSON()
			case FormatJSONL:
				content, err = history.ExportJSONL()
			default:
				content = []byte(history.ForExport().Render())
			}

			if err != nil {
				return "", fmt.Errorf("exporting: %w", err)
			}

			if dest == "clipboard" {
				clipboard.Copy(string(content))

				return fmt.Sprintf("Conversation (%s) copied to clipboard", formatName(format)), nil
			}

			if err := file.New(dest).Write(content); err != nil {
				return "", fmt.Errorf("writing export: %w", err)
			}

			return fmt.Sprintf("Conversation exported to %s (%s)", dest, formatName(format)), nil
		},
	}
}

// parseExportArgs extracts the format and destination from command arguments.
//
// Rules:
//  1. No args or "clipboard" → plaintext to clipboard
//  2. First arg is a format keyword → explicit format; rest is destination (or clipboard)
//  3. Otherwise → destination from args; format inferred from extension; default plaintext
func parseExportArgs(args []string) (Format, string) {
	if len(args) == 0 {
		return FormatPlaintext, "clipboard"
	}

	if args[0] == "clipboard" {
		return FormatPlaintext, "clipboard"
	}

	// Check if first arg is an explicit format keyword.
	if fmt, ok := formatKeywords[strings.ToLower(args[0])]; ok {
		dest := "clipboard"

		if len(args) > 1 {
			dest = strings.Join(args[1:], " ")
		}

		return fmt, dest
	}

	// Destination from all args; infer format from extension.
	dest := strings.Join(args, " ")
	ext := strings.ToLower(file.New(dest).Extension())

	if fmt, ok := extensionFormats[ext]; ok {
		return fmt, dest
	}

	return FormatPlaintext, dest
}

// formatName returns a human-readable name for the format.
func formatName(f Format) string {
	switch f {
	case FormatMarkdown:
		return "markdown"
	case FormatJSON:
		return "json"
	case FormatJSONL:
		return "jsonl"
	default:
		return "plaintext"
	}
}
