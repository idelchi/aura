package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/MakeNowJust/heredoc/v2"
	cli "github.com/urfave/cli/v3"

	"github.com/idelchi/aura/internal/assistant"
	"github.com/idelchi/aura/internal/cli/core"
	"github.com/idelchi/aura/internal/ui"
)

// Command creates the 'aura run' command for non-interactive prompt execution.
func Command(flags *core.Flags) *cli.Command {
	return &cli.Command{
		Name:  "run",
		Usage: "Execute prompts non-interactively",
		Description: heredoc.Doc(`
			Run executes prompts through the assistant and exits.

			Multiple arguments are treated as separate turns — the assistant
			responds to each before processing the next. A single argument
			is a single turn. Prompts can also be piped via stdin.

			Use --timeout to set a maximum execution time for all prompts.
			A timeout of 0 (the default) means no limit.
		`) + "\n\nExamples:\n" + heredoc.Doc(`
			# Single prompt
			aura run "Write a Go function that parses CSV files"

			# Multi-turn
			aura run "/verbose true" "/mode plan" "Generate a plan for auth"

			# Piped input
			echo "Explain this error" | aura run

			# With agent override
			aura --agent high run "Summarize the changes in git diff"

			# With timeout
			aura run --timeout 2m "Summarize the codebase"
		`),
		Flags: []cli.Flag{
			&cli.DurationFlag{
				Name:        "timeout",
				Usage:       "Maximum execution time for all prompts (0 = unlimited)",
				Value:       0,
				Destination: &flags.Run.Timeout,
				Sources:     cli.EnvVars("AURA_RUN_TIMEOUT"),
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := core.GetFlags()

			prompts := cmd.Args().Slice()

			if len(prompts) == 0 {
				fi, err := os.Stdin.Stat()
				if err == nil && fi.Mode()&os.ModeCharDevice == 0 {
					data, err := io.ReadAll(os.Stdin)
					if err != nil {
						return err
					}

					if p := strings.TrimSpace(string(data)); p != "" {
						prompts = []string{p}
					}
				}
			}

			if len(prompts) == 0 {
				return errors.New("no prompt provided (pass as argument or pipe via stdin)")
			}

			timeout := flags.Run.Timeout

			if flags.Dry == "render" {
				fmt.Fprintln(cmd.Writer, "User prompts:")

				for i, p := range prompts {
					fmt.Fprintf(cmd.Writer, "  [%d] %s\n", i+1, p)
				}

				fmt.Fprintln(cmd.Writer)
			}

			return core.RunSession(
				flags,
				core.HeadlessUI,
				func(ctx context.Context, _ context.CancelCauseFunc, asst *assistant.Assistant, u ui.UI) error {
					go u.Run(ctx)

					u.Events() <- ui.StatusChanged{Status: asst.Status()}

					u.Events() <- ui.DisplayHintsChanged{Hints: asst.DisplayHints()}

					if timeout > 0 {
						var cancel context.CancelFunc

						ctx, cancel = context.WithTimeout(ctx, timeout)

						defer cancel()
					}

					for _, prompt := range prompts {
						if ctx.Err() != nil {
							break
						}

						if err := asst.ProcessInput(ctx, prompt); err != nil {
							return err
						}

						if ctx.Err() == nil {
							done := make(chan struct{})

							select {
							case u.Events() <- ui.Flush{Done: done}:
								select {
								case <-done:
								case <-ctx.Done():
								}
							case <-ctx.Done():
							}
						}
					}

					return ctx.Err()
				},
			)
		},
	}
}
