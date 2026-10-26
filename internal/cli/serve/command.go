package serve

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/MakeNowJust/heredoc/v2"
	cli "github.com/urfave/cli/v3"

	"github.com/idelchi/aura/internal/assistant"
	"github.com/idelchi/aura/internal/cli/core"
	"github.com/idelchi/aura/internal/serve"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/automation"
)

// tokenEnv carries the API token. It is removed from the environment before any child starts.
const tokenEnv = "AURA_SERVE_TOKEN"

// Command creates the 'aura serve' command.
func Command(_ *core.Flags) *cli.Command {
	return &cli.Command{
		Name:  "serve",
		Usage: "Serve the automation API (/v1) for one session",
		Description: heredoc.Doc(`
			Serves one session over HTTP: create a fresh session, or continue one with
			the root --resume flag. Runs are submitted, observed and cancelled through
			/v1; every event is journaled and replayable. See docs/features/automation.md.

			The bearer token is read from AURA_SERVE_TOKEN (at least 32 characters)
			and removed from the environment before any tool or MCP server starts.

			The ready file receives {"api","url","pid","session_id",...} once the
			session is saved, required MCP servers are connected and the server
			listens — or {"api","error":{"code","message"}} if startup fails.
		`) + "\n\nExamples:\n" + heredoc.Doc(`
			AURA_SERVE_TOKEN=$(openssl rand -hex 32) aura --resume task-1 serve --ready-file /tmp/ready.json
		`),
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "bind", Usage: "Address to listen on", Value: "127.0.0.1:0"},
			&cli.StringFlag{Name: "ready-file", Usage: "File receiving the server address or startup error", Required: true},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			flags := core.GetFlags()
			readyFile := cmd.String("ready-file")

			token := os.Getenv(tokenEnv)
			os.Unsetenv(tokenEnv)

			if len(token) < 32 {
				err := fmt.Errorf("%s must hold at least 32 characters", tokenEnv)

				return report(readyFile, automation.NewError(automation.CodeUnauthorized, "%v", err), err)
			}

			if flags.Dry == "render" {
				err := errors.New("--dry=render cannot serve a session")

				return report(readyFile, automation.NewError(automation.CodeBadRequest, "%v", err), err)
			}

			api := serve.NewUI()

			var server *serve.Server

			work := func(ctx context.Context, cancel context.CancelCauseFunc, asst *assistant.Assistant, u ui.UI) error {
				var err error

				server, err = serve.Start(serve.Options{
					Assistant: asst,
					UI:        api,
					Bind:      cmd.String("bind"),
					ReadyFile: readyFile,
					Token:     token,
					Version:   cmd.Root().Version,
					Commit:    commit(),
				})
				if err != nil {
					server = nil

					return err
				}

				// The engine outlives the signal: shutdown first settles the active run.
				engineCtx, stopEngine := context.WithCancel(context.WithoutCancel(ctx))

				go func() {
					select {
					case <-ctx.Done():
						server.Shutdown("signal")
					case reason := <-server.ShutdownRequested():
						server.Shutdown(reason)
					}

					stopEngine()
				}()

				err = core.RunInteractive(engineCtx, cancel, asst, u)

				return errors.Join(err, server.Close())
			}

			err := core.RunSession(ctx, flags, core.Selection{}, func(core.Flags) (ui.UI, error) { return api, nil }, work)
			if err != nil && server == nil {
				return report(readyFile, startupError(err), err)
			}

			return err
		},
	}
}

// report writes a startup failure to the ready file and returns err.
func report(readyFile string, failure *automation.Error, err error) error {
	if writeErr := serve.WriteReady(readyFile, automation.Ready{API: automation.APIVersion, Error: failure}); writeErr != nil {
		return errors.Join(err, writeErr)
	}

	return err
}

// startupError classifies why the session could not be served. A provider failure keeps
// its own code and details whatever startup step met it (e.g. restoring a model), so a
// transient outage stays retryable and distinct from configuration or credential faults.
func startupError(err error) *automation.Error {
	failure := serve.Classify(err)

	switch failure.Code {
	case automation.CodeProviderNetwork, automation.CodeProviderServer, automation.CodeProviderRateLimit,
		automation.CodeProviderAuth, automation.CodeProviderCredit, automation.CodeProviderModelUnavailable,
		automation.CodeContentFilter, automation.CodeContextExhausted:
		return failure
	}

	switch {
	case errors.Is(err, core.ErrMCPUnavailable):
		failure.Code = automation.CodeMCPUnavailable
	case errors.Is(err, core.ErrResume):
		failure.Code = automation.CodeResumeFailed
	case errors.Is(err, core.ErrConfig):
		failure.Code = automation.CodeConfigInvalid
	default:
		switch failure.Code {
		case automation.CodeSessionFrozen, automation.CodeSessionLocked, automation.CodeNotFound,
			automation.CodeInvalidID, automation.CodeInvalidSession:
		default:
			failure.Code = automation.CodeStartupFailed
		}
	}

	failure.Retryable = failure.Code.Retryable()

	return failure
}

// commit returns the VCS revision the binary was built from ("+dirty" when modified).
func commit() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}

	var revision string

	dirty := false

	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			dirty = setting.Value == "true"
		}
	}

	if dirty && revision != "" {
		revision += "+dirty"
	}

	return revision
}
