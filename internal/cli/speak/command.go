package speak

import (
	"context"
	"errors"

	"github.com/MakeNowJust/heredoc/v2"
	cli "github.com/urfave/cli/v3"

	"github.com/idelchi/aura/internal/cli/core"
)

// Command creates the 'aura speak' subcommand for text-to-speech.
func Command(flags *core.Flags) *cli.Command {
	return &cli.Command{
		Name:  "speak",
		Usage: "Convert text to speech audio",
		Description: heredoc.Doc(`
			Convert text to speech using an OpenAI-compatible TTS server.

			Writes the audio output to the specified file path.
			Requires a TTS agent configured in features/tts.yaml.

			Examples:
			  # Basic text-to-speech
			  aura speak "Hello, world!" hello.mp3

			  # With a specific voice
			  aura speak "Good morning" greeting.mp3 --voice af_heart
		`),
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "voice",
				Aliases:     []string{"V"},
				Usage:       "Voice identifier (e.g. alloy, af_heart)",
				Destination: &flags.Speak.Voice,
				Sources:     cli.EnvVars("AURA_SPEAK_VOICE"),
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() != 2 {
				return errors.New("expected 2 arguments")
			}

			flags := core.GetFlags()
			toolArgs := map[string]any{
				"text":        cmd.Args().Get(0),
				"output_path": cmd.Args().Get(1),
			}

			if flags.Speak.Voice != "" {
				toolArgs["voice"] = flags.Speak.Voice
			}

			return core.RunSingleTool(cmd.Writer, "Speak", toolArgs)
		},
	}
}
