package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/todo"
	"github.com/idelchi/aura/internal/tools"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/aura/pkg/spinner"
)

// RunSingleTool handles the full lifecycle of a single-tool CLI command:
// flags, config, tool registry, lookup, execute, print output.
// Used by vision, transcribe, and speak commands.
func RunSingleTool(w io.Writer, toolName string, toolArgs map[string]any) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	flags := GetFlags()

	debug.Init(flags.WriteHome(), flags.Debug)

	defer debug.Close()

	cfg, paths, err := config.New(flags.ConfigOptions(),
		config.PartFeatures, config.PartToolDefs, config.PartSkills, config.PartProviders,
		config.PartAgents, config.PartModes, config.PartSystems, config.PartAgentsMd,
	)
	if err != nil {
		return err
	}

	eventsCh := make(chan ui.Event, 100)

	allTools, err := tools.All(cfg, paths, &config.Runtime{}, todo.New(), eventsCh, nil, nil, nil)
	if err != nil {
		return err
	}

	t, err := allTools.Get(toolName)
	if err != nil {
		return fmt.Errorf("%s tool not available: %w", toolName, err)
	}

	output, err := ExecuteTool(ctx, t, toolArgs, eventsCh, true)
	if err != nil {
		return err
	}

	fmt.Fprintln(w, output)

	return nil
}

// ExecuteTool runs a tool with a spinner and events bridge when interactive.
// When interactive is true, a spinner shows progress and tool-emitted SpinnerMessage
// events update the spinner text. When false, the tool runs silently.
func ExecuteTool(
	ctx context.Context,
	t tool.Tool,
	args map[string]any,
	eventsCh chan ui.Event,
	interactive bool,
) (string, error) {
	var s *spinner.Spinner

	if interactive {
		s = spinner.New("Executing " + t.Name() + "...")

		go func() {
			for ev := range eventsCh {
				if sm, ok := ev.(ui.SpinnerMessage); ok {
					s.Update(sm.Text)
				}
			}
		}()

		s.Start()
	}

	// In headless mode (sandbox re-exec), inject a streaming callback that
	// writes prefixed lines to stderr. The parent process reads these and
	// emits ToolOutputDelta events to the UI.
	if !interactive {
		ctx = tool.WithStreamCallback(ctx, func(line string) {
			fmt.Fprintf(os.Stderr, "\x00STREAM:%s\n", line)
		})
	}

	output, err := t.Execute(ctx, args)

	if interactive {
		close(eventsCh)
		s.Stop()
	}

	return output, err
}

// OutputResult handles both plain and JSON output for tool results.
func OutputResult(w io.Writer, jsonOutput bool, output string, err error) error {
	if jsonOutput {
		tr := tools.ToolResult{Stdout: output}

		if err != nil {
			tr.ExitCode = 1
			tr.Error = err.Error()
		}

		return json.NewEncoder(w).Encode(tr)
	}

	// Plain output
	if output != "" {
		fmt.Fprintln(w, output)
	}

	if err != nil {
		return fmt.Errorf("tool execution: %w", err)
	}

	return nil
}

// OutputError handles error output in both plain and JSON formats.
func OutputError(w io.Writer, jsonOutput bool, err error) error {
	if jsonOutput {
		tr := tools.ToolResult{ExitCode: 1, Error: err.Error()}

		return json.NewEncoder(w).Encode(tr)
	}

	return err
}

// OutputSetupError handles setup/infrastructure errors (config, plugins, estimator).
// Like OutputError, but marks the result as a setup error so the parent process
// can route it to the user instead of the LLM.
func OutputSetupError(w io.Writer, jsonOutput bool, err error) error {
	if jsonOutput {
		tr := tools.ToolResult{ExitCode: 1, Error: err.Error(), Setup: true}

		return json.NewEncoder(w).Encode(tr)
	}

	return err
}
