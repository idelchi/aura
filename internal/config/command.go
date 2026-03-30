package config

import (
	"fmt"
	"strings"

	"github.com/idelchi/aura/pkg/frontmatter"
	"github.com/idelchi/godyl/pkg/path/files"
)

// CustomCommand represents a user-defined slash command loaded from a Markdown file.
type CustomCommand struct {
	Metadata struct {
		Name        string `validate:"required"`
		Description string
		Hints       string // Argument hint text shown as ghost text in TUI (e.g. "<file> [options]").
	}
	Body string
}

// Name returns the command's identifier.
func (c CustomCommand) Name() string { return c.Metadata.Name }

// loadCommands parses custom command Markdown files and returns a Collection keyed by source file.
func loadCommands(ff files.Files) (Collection[CustomCommand], error) {
	result := make(Collection[CustomCommand])

	for _, file := range ff {
		var cmd CustomCommand

		body, err := frontmatter.Load(file, &cmd.Metadata)
		if err != nil {
			return nil, fmt.Errorf("command %s: %w", file, err)
		}

		cmd.Body = strings.TrimSpace(body)

		for k, existing := range result {
			if strings.EqualFold(existing.Metadata.Name, cmd.Metadata.Name) {
				delete(result, k)

				break
			}
		}

		result[file] = cmd
	}

	return result, nil
}

// Populate performs argument substitution on a command body.
// Positional placeholders ($1, $2, ...) are replaced in reverse order
// to prevent $1 from clobbering $12. $ARGUMENTS is replaced with the
// full joined argument string.
func Populate(body string, args ...string) string {
	for i := len(args) - 1; i >= 0; i-- {
		placeholder := fmt.Sprintf("$%d", i+1)

		body = strings.ReplaceAll(body, placeholder, args[i])
	}

	body = strings.ReplaceAll(body, "$ARGUMENTS", strings.Join(args, " "))

	return body
}
