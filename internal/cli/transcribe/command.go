package transcribe

import (
	"context"
	"errors"

	"github.com/MakeNowJust/heredoc/v2"
	cli "github.com/urfave/cli/v3"

	"github.com/idelchi/aura/internal/cli/core"
)

// Command creates the 'aura transcribe' subcommand for speech-to-text.
func Command(flags *core.Flags) *cli.Command {
	return &cli.Command{
		Name:  "transcribe",
		Usage: "Transcribe an audio file to text",
		Description: heredoc.Doc(`
			Transcribe an audio file using a whisper-compatible STT server.

			Supports common audio formats: mp3, wav, ogg, flac, m4a, webm.
			Requires an STT agent configured in features/stt.yaml.

			Examples:
			  # Transcribe with auto-detected language
			  aura transcribe recording.mp3

			  # Transcribe with language hint
			  aura transcribe meeting.wav --language en

			  # Japanese audio
			  aura transcribe notes.ogg --language ja
		`),
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "language",
				Aliases:     []string{"l"},
				Usage:       "ISO-639-1 language hint (e.g. en, de, ja)",
				Destination: &flags.Transcribe.Language,
				Sources:     cli.EnvVars("AURA_TRANSCRIBE_LANGUAGE"),
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() != 1 {
				return errors.New("expected 1 argument")
			}

			flags := core.GetFlags()
			toolArgs := map[string]any{"path": cmd.Args().Get(0)}

			if flags.Transcribe.Language != "" {
				toolArgs["language"] = flags.Transcribe.Language
			}

			return core.RunSingleTool(cmd.Writer, "Transcribe", toolArgs)
		},
	}
}
