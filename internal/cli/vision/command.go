package vision

import (
	"context"
	"errors"

	"github.com/MakeNowJust/heredoc/v2"
	cli "github.com/urfave/cli/v3"

	"github.com/idelchi/aura/internal/cli/core"
)

// Command creates the 'aura vision' subcommand for image/PDF analysis.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "vision",
		Usage: "Analyze an image or PDF via a vision-capable LLM",
		Description: heredoc.Doc(`
			Send an image or PDF to a vision-capable LLM for analysis.

			If no instruction is given, the model extracts text or describes
			what it sees. Requires a vision agent configured in features/vision.yaml.

			Examples:
			  # Describe an image
			  aura vision screenshot.png

			  # Extract text from a PDF
			  aura vision document.pdf "Extract all text from this document"

			  # Analyze a diagram
			  aura vision diagram.jpg "Describe the architecture shown here"
		`),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			n := cmd.Args().Len()
			if n < 1 || n > 2 {
				return errors.New("expected 1 or 2 arguments")
			}

			toolArgs := map[string]any{"path": cmd.Args().Get(0)}

			if n > 1 {
				toolArgs["instruction"] = cmd.Args().Get(1)
			}

			return core.RunSingleTool(cmd.Writer, "Vision", toolArgs)
		},
	}
}
