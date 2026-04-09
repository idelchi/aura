package tokens

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/signal"
	"slices"
	"strings"

	"github.com/MakeNowJust/heredoc/v2"
	cli "github.com/urfave/cli/v3"

	"github.com/idelchi/aura/internal/agent"
	"github.com/idelchi/aura/internal/cli/core"
	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/todo"
	"github.com/idelchi/aura/internal/tools"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/request"
	pkgproviders "github.com/idelchi/aura/pkg/providers"
)

// Command creates the 'aura tokens' command for token counting.
func Command(flags *core.Flags) *cli.Command {
	return &cli.Command{
		Name:  "tokens",
		Usage: "Count tokens in a file or stdin",
		Description: heredoc.Doc(`
			Count tokens using the configured estimation method.
			Reads from a file path argument or stdin.

			The estimation method defaults to the value in
			features/estimation.yaml. Override with --method.
			Multiple methods can be specified for comparison.

			Native estimation uses the current agent's provider.
			Specify --agent/--provider/--model to control which
			provider is used, or omit to use the default agent.
		`) + "\n\nExamples:\n" + heredoc.Doc(`
			# Count tokens in a file
			aura tokens path/to/file.go

			# Count tokens from stdin
			cat large-prompt.txt | aura tokens

			# Override estimation method
			aura tokens --method tiktoken path/to/file.go

			# Compare multiple methods
			aura tokens --method rough --method tiktoken path/to/file.go

			# Native estimation (uses default agent's provider)
			aura tokens --method native path/to/file.go

			# Native with explicit provider/model
			aura --provider ollama --model qwen3:32b tokens --method native path/to/file.go

			# All methods at once
			aura tokens --method rough --method tiktoken --method rough+tiktoken --method native path/to/file.go
		`),
		Before: func(_ context.Context, _ *cli.Command) (context.Context, error) {
			flags := core.GetFlags()
			debug.Init(flags.WriteHome(), flags.Debug)

			return nil, nil
		},
		After: func(_ context.Context, _ *cli.Command) error {
			debug.Close()

			return nil
		},
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:        "method",
				Usage:       "Estimation method (rough, tiktoken, rough+tiktoken, native; repeatable)",
				Destination: &flags.Tokens.Method,
				Sources:     cli.EnvVars("AURA_TOKENS_METHOD"),
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := core.GetFlags()

			// Resolve input: file path or stdin content.
			toolArgs, err := resolveInput(cmd.Args().First(), cmd.Args().Len() > 0)
			if err != nil {
				return err
			}

			// Load config (agents/modes/providers needed for native estimation).
			cfg, paths, err := config.New(flags.ConfigOptions(),
				config.PartFeatures, config.PartToolDefs, config.PartProviders,
				config.PartAgents, config.PartModes, config.PartSystems, config.PartAgentsMd,
			)
			if err != nil {
				return err
			}

			// Resolve methods: explicit --method flag or configured default.
			methods := flags.Tokens.Method
			if len(methods) == 0 {
				methods = []string{cfg.Features.Estimation.Method}
			}

			// Create estimator and set on runtime for the Tokens tool.
			estimator, err := cfg.Features.Estimation.NewEstimator()
			if err != nil {
				return err
			}

			// Wire native estimation from CLI flags if needed.
			if hasNative(methods) {
				if err := wireNative(cfg, paths, flags, estimator); err != nil {
					return err
				}
			}

			rt := &config.Runtime{Estimator: estimator}
			eventsCh := make(chan ui.Event, 100)

			allTools, err := tools.All(cfg, paths, rt, todo.New(), eventsCh, nil, nil, estimator.EstimateLocal)
			if err != nil {
				return err
			}

			t, err := allTools.Get("Tokens")
			if err != nil {
				return fmt.Errorf("Tokens tool not available: %w", err)
			}

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()

			single := len(methods) == 1

			for _, m := range methods {
				args := make(map[string]any, len(toolArgs)+1)
				maps.Copy(args, toolArgs)

				args["method"] = m

				output, err := t.Execute(ctx, args)
				if err != nil {
					return err
				}

				if single {
					fmt.Fprintln(cmd.Writer, output)
				} else {
					fmt.Fprintf(cmd.Writer, "%s: %s\n", m, output)
				}
			}

			return nil
		},
	}
}

// hasNative reports whether any method in the list is "native".
func hasNative(methods []string) bool {
	return slices.Contains(methods, "native")
}

// wireNative resolves the agent from CLI flags and wires UseNative on the estimator.
// Follows the same agent resolution as session startup: --agent or default, with --provider/--model overrides.
func wireNative(cfg config.Config, paths config.Paths, flags core.Flags, estimator interface {
	UseNative(func(context.Context, string) (int, error))
},
) error {
	agentName := flags.Agent
	if agentName == "" {
		resolved, err := config.ResolveDefault(cfg.Agents, flags.Homes())
		if err != nil {
			return fmt.Errorf("resolving default agent for native estimation: %w", err)
		}

		agentName = resolved.Metadata.Name
	}

	var overrides agent.Overrides

	if flags.IsSet("provider") {
		overrides.Provider = &flags.Provider
	}

	if flags.IsSet("model") {
		overrides.Model = &flags.Model
	}

	ag, err := agent.New(cfg, paths, &config.Runtime{}, agentName, overrides)
	if err != nil {
		return fmt.Errorf("creating agent for native estimation: %w", err)
	}

	estimator.UseNative(func(ctx context.Context, text string) (int, error) {
		resolved, err := ag.Provider.Model(ctx, ag.Model.Name)
		if err != nil {
			return 0, fmt.Errorf("resolving model %q: %w", ag.Model.Name, err)
		}

		req := request.Request{
			Model:         resolved,
			ContextLength: ag.Model.Context,
		}

		count, err := ag.Provider.Estimate(ctx, req, text)
		if err != nil {
			if errors.Is(err, pkgproviders.ErrContextExhausted) {
				return count, nil
			}

			return 0, err
		}

		return count, nil
	})

	return nil
}

// resolveInput determines the tool arguments from a file path or stdin.
func resolveInput(path string, hasArg bool) (map[string]any, error) {
	if hasArg {
		return map[string]any{"path": path}, nil
	}

	fi, err := os.Stdin.Stat()
	if err == nil && fi.Mode()&os.ModeCharDevice == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, err
		}

		content := strings.TrimSpace(string(data))
		if content == "" {
			return nil, errors.New("empty input from stdin")
		}

		return map[string]any{"content": content}, nil
	}

	return nil, errors.New("no input: provide a file path or pipe via stdin")
}
