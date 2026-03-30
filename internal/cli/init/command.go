package initialize

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"

	cli "github.com/urfave/cli/v3"

	"github.com/idelchi/aura/internal/cli/core"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// Command creates the init command to extract embedded configuration.
func Command(flags *core.Flags) *cli.Command {
	return &cli.Command{
		Name:        "init",
		Usage:       "Initialize a default aura configuration",
		Description: "Extract the embedded default configuration to the specified directory.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "dir",
				Aliases:     []string{"d"},
				Usage:       "Output directory for configuration",
				Value:       ".aura",
				Destination: &flags.Init.Dir,
				Sources:     cli.EnvVars("AURA_INIT_DIR"),
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Present() {
				return errors.New("unexpected arguments")
			}

			flags := core.GetFlags()

			d := folder.New(flags.Init.Dir)
			if !d.IsAbs() {
				d = folder.New(core.LaunchDir, flags.Init.Dir)
			}

			dir := d.Path()

			return extractEmbedded(cmd.Writer, dir)
		},
	}
}

func extractEmbedded(w io.Writer, dest string) error {
	sub, err := fs.Sub(core.EmbeddedConfig, "samples")
	if err != nil {
		return err
	}

	return fs.WalkDir(sub, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if path == "." {
			return nil
		}

		dst := file.New(dest, path)

		if d.IsDir() {
			return folder.New(dst.Path()).Create()
		}

		if dst.Exists() {
			fmt.Fprintf(w, "  %s (skipped)\n", dst)

			return nil
		}

		content, err := fs.ReadFile(sub, path)
		if err != nil {
			return err
		}

		fmt.Fprintf(w, "  %s\n", dst)

		return dst.Write(content)
	})
}
