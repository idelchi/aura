package directive

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

var bashPattern = regexp.MustCompile(`@Bash\[([^\]]+)\]`)

// parseShell processes all @Bash[command] directives in the input.
// Command outputs are collected into preamble blocks.
// The token in the user text is replaced with "[Command: cmd]".
//
// If runBash is nil and @Bash tokens are present, each token is left
// un-replaced and a warning is emitted (no silent fallback).
func parseShell(
	ctx context.Context,
	input string,
	runBash func(ctx context.Context, command string) (string, error),
) (string, string, []string) {
	var blocks []string

	var warnings []string

	if runBash == nil {
		// No executor available — warn for each @Bash token, leave un-replaced.
		for _, match := range bashPattern.FindAllStringSubmatch(input, -1) {
			warnings = append(warnings, fmt.Sprintf("@Bash[%s]: no bash executor configured", match[1]))
		}

		return input, "", warnings
	}

	text := bashPattern.ReplaceAllStringFunc(input, func(match string) string {
		subs := bashPattern.FindStringSubmatch(match)
		command := subs[1]

		output, err := runBash(ctx, command)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("@Bash[%s]: %v", command, err))

			if output != "" {
				blocks = append(blocks, fmt.Sprintf("### Command: %s\nOutput (error):\n%s", command, output))
			}
		} else {
			blocks = append(blocks, fmt.Sprintf("### Command: %s\nOutput:\n%s", command, output))
		}

		return fmt.Sprintf("[Command: %s]", command)
	})

	preamble := strings.Join(blocks, "\n\n")

	return text, preamble, warnings
}
