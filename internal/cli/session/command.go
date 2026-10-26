package session

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/MakeNowJust/heredoc/v2"
	cli "github.com/urfave/cli/v3"

	"github.com/idelchi/aura/internal/cli/core"
	"github.com/idelchi/aura/internal/serve"
	"github.com/idelchi/aura/internal/session"
	"github.com/idelchi/aura/pkg/automation"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// Command creates the 'aura session' command for offline session store operations.
func Command(_ *core.Flags) *cli.Command {
	jsonFlag := &cli.BoolFlag{Name: "json", Usage: "Print one JSON object (errors as JSON on stderr)"}

	return &cli.Command{
		Name:  "session",
		Usage: "Inspect, import, export, fork and freeze stored sessions",
		Description: heredoc.Doc(`
			Operates on the session store of the write home (the last --config
			directory, else --home): <write home>/sessions. No model or
			assistant is started.

			A frozen session is immutable: it can be shown, exported and forked,
			never resumed or saved. A fork records the exact parent revision
			(sha256 of the stored file) and never writes the parent.
			Import, export, fork and freeze are idempotent.
		`),
		Commands: []*cli.Command{
			{
				Name:      "list",
				Usage:     "List stored sessions, newest first",
				Flags:     []cli.Flag{jsonFlag},
				Action:    action(listSessions),
				ArgsUsage: " ",
			},
			{
				Name:      "show",
				Usage:     "Describe one stored session",
				ArgsUsage: "<id>",
				Flags:     []cli.Flag{jsonFlag},
				Action:    action(showSession),
			},
			{
				Name:      "import",
				Usage:     "Store a native session file",
				ArgsUsage: "<file>",
				Flags: []cli.Flag{
					jsonFlag,
					&cli.StringFlag{Name: "id", Usage: "Store under this ID instead of the file's own"},
					&cli.BoolFlag{Name: "freeze", Usage: "Freeze the imported session"},
				},
				Action: action(importSession),
			},
			{
				Name:      "export",
				Usage:     "Copy the stored bytes of a session to a file",
				ArgsUsage: "<id>",
				Flags: []cli.Flag{
					jsonFlag,
					&cli.StringFlag{Name: "output", Usage: "Destination file", Required: true},
				},
				Action: action(exportSession),
			},
			{
				Name:      "fork",
				Usage:     "Store an independent copy of a session",
				ArgsUsage: "<parent>",
				Flags: []cli.Flag{
					jsonFlag,
					&cli.StringFlag{Name: "id", Usage: "Child session ID", Required: true},
					&cli.StringFlag{Name: "title", Usage: "Child title (default: the parent's)"},
					&cli.StringFlag{Name: "expect-revision", Usage: "Fail unless the parent has this revision"},
				},
				Action: action(forkSession),
			},
			{
				Name:      "freeze",
				Usage:     "Make a session permanently immutable",
				ArgsUsage: "<id>",
				Flags: []cli.Flag{
					jsonFlag,
					&cli.StringFlag{Name: "expect-revision", Usage: "Fail unless the session has this revision"},
				},
				Action: action(freezeSession),
			},
		},
	}
}

// operation runs one store operation and returns its JSON result and human summary.
type operation func(store *session.Store, cmd *cli.Command) (any, string, error)

// action wraps an operation with store construction and JSON or text output.
func action(op operation) cli.ActionFunc {
	return func(_ context.Context, cmd *cli.Command) error {
		flags := core.GetFlags()
		store := session.NewStore(folder.New(flags.WriteHome(), "sessions").Path())

		result, summary, err := op(store, cmd)

		if !cmd.Bool("json") {
			if err != nil {
				return err
			}

			fmt.Fprintln(cmd.Writer, summary)

			return nil
		}

		if err != nil {
			report(cmd.ErrWriter, err)

			return fmt.Errorf("%w: %w", core.ErrReported, err)
		}

		encoder := json.NewEncoder(cmd.Writer)
		encoder.SetIndent("", "  ")

		return encoder.Encode(result)
	}
}

// report writes err as an automation error envelope.
func report(w io.Writer, err error) {
	if w == nil {
		w = os.Stderr
	}

	data, _ := json.Marshal(automation.Envelope{Error: serve.Classify(err)})

	fmt.Fprintln(w, string(data))
}

// argument returns the single positional argument.
func argument(cmd *cli.Command, name string) (string, error) {
	if cmd.Args().Len() != 1 {
		return "", automation.NewError(automation.CodeBadRequest, "expected exactly one %s argument", name)
	}

	return cmd.Args().First(), nil
}

func listSessions(store *session.Store, cmd *cli.Command) (any, string, error) {
	if cmd.Args().Present() {
		return nil, "", automation.NewError(automation.CodeBadRequest, "list takes no arguments")
	}

	result, err := store.List()
	if err != nil {
		return nil, "", err
	}

	list := automation.SessionList{Sessions: result.Sessions, Warnings: result.Warnings}
	if list.Sessions == nil {
		list.Sessions = []automation.SessionSummary{}
	}

	if list.Warnings == nil {
		list.Warnings = []string{}
	}

	text := ""
	for _, w := range result.Warnings {
		text += "warning: " + w + "\n"
	}

	for _, s := range result.Sessions {
		text += describe(s) + "\n"
	}

	if text == "" {
		text = "No saved sessions\n"
	}

	return list, text[:len(text)-1], nil
}

func showSession(store *session.Store, cmd *cli.Command) (any, string, error) {
	id, err := argument(cmd, "session id")
	if err != nil {
		return nil, "", err
	}

	info, err := store.Info(id)
	if err != nil {
		return nil, "", err
	}

	return info, describe(info), nil
}

func importSession(store *session.Store, cmd *cli.Command) (any, string, error) {
	path, err := argument(cmd, "file")
	if err != nil {
		return nil, "", err
	}

	result, err := store.Import(path, cmd.String("id"), cmd.Bool("freeze"))
	if err != nil {
		return nil, "", err
	}

	verb := "Already imported"
	if result.Created {
		verb = "Imported"
	}

	return result, verb + ": " + describe(result.Session), nil
}

func exportSession(store *session.Store, cmd *cli.Command) (any, string, error) {
	id, err := argument(cmd, "session id")
	if err != nil {
		return nil, "", err
	}

	result, err := store.Export(id, cmd.String("output"))
	if err != nil {
		return nil, "", err
	}

	verb := "Already exported"
	if result.Written {
		verb = "Exported"
	}

	return result, fmt.Sprintf("%s %s to %s", verb, id, result.Path), nil
}

func forkSession(store *session.Store, cmd *cli.Command) (any, string, error) {
	parent, err := argument(cmd, "parent session id")
	if err != nil {
		return nil, "", err
	}

	result, err := store.Fork(parent, cmd.String("id"), cmd.String("title"), cmd.String("expect-revision"))
	if err != nil {
		return nil, "", err
	}

	verb := "Already forked"
	if result.Created {
		verb = "Forked"
	}

	return result, verb + ": " + describe(result.Session), nil
}

func freezeSession(store *session.Store, cmd *cli.Command) (any, string, error) {
	id, err := argument(cmd, "session id")
	if err != nil {
		return nil, "", err
	}

	result, err := store.Freeze(id, cmd.String("expect-revision"))
	if err != nil {
		return nil, "", err
	}

	verb := "Already frozen"
	if result.Changed {
		verb = "Frozen"
	}

	return result, verb + ": " + describe(result.Session), nil
}

// describe formats a session summary for humans.
func describe(s automation.SessionSummary) string {
	line := fmt.Sprintf("%s  rev %s  %d messages", s.ID, s.Revision[:12], s.Messages)

	if s.Title != "" {
		line += "  [" + s.Title + "]"
	}

	if s.Frozen {
		line += "  frozen"
	}

	if s.Parent != nil {
		line += fmt.Sprintf("  fork of %s@%s", s.Parent.ID, s.Parent.Revision[:12])
	}

	return line
}
