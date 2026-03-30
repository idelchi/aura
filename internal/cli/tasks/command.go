package tasks

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"golang.org/x/sync/errgroup"

	"github.com/MakeNowJust/heredoc/v2"
	cli "github.com/urfave/cli/v3"

	"github.com/idelchi/aura/internal/assistant"
	"github.com/idelchi/aura/internal/cli/core"
	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/task"
	"github.com/idelchi/aura/internal/ui"
)

// Command creates the 'aura tasks' command with the run subcommand.
func Command(flags *core.Flags) *cli.Command {
	return &cli.Command{
		Name:  "tasks",
		Usage: "Manage and run scheduled tasks",
		Description: heredoc.Doc(`
			Manage scheduled tasks defined in .aura/config/tasks/*.yaml.

			Tasks are named sets of commands (slash commands and prompts)
			that can be run manually or on a cron schedule.
		`),
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:        "files",
				Usage:       "Additional task file globs to load (repeatable, supports ** syntax)",
				Sources:     cli.EnvVars("AURA_TASKS_FILES"),
				Destination: &flags.Tasks.Files,
			},
		},
		Before: func(_ context.Context, _ *cli.Command) (context.Context, error) {
			flags := core.GetFlags()
			debug.Init(flags.WriteHome(), flags.Debug)

			return nil, nil
		},
		After: func(_ context.Context, _ *cli.Command) error {
			debug.Close()

			return nil
		},
		Commands: []*cli.Command{
			run(flags),
		},
	}
}

func run(flags *core.Flags) *cli.Command {
	return &cli.Command{
		Name:  "run",
		Usage: "Run tasks immediately or on schedule",
		Description: heredoc.Doc(`
			Run tasks through the assistant.

			By default, starts a scheduler daemon that executes tasks on their
			configured schedules. Tasks without a schedule are skipped.
			The daemon runs until interrupted (SIGINT/SIGTERM).

			With --now, runs tasks immediately and exits. When names are given,
			runs those specific tasks. Without names, runs all scheduled tasks.

			Use --prepend and --append to inject commands before/after each
			task's command list. Use --start to resume from a specific index
			(skips commands for regular tasks, skips items for foreach tasks).

			Root flags --agent and --mode take precedence over task YAML when
			explicitly set on the command line.
		`),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "now",
				Usage:       "Run tasks immediately instead of on schedule",
				Destination: &flags.Tasks.Run.Now,
			},
			&cli.UintFlag{
				Name:        "concurrency",
				Usage:       "Maximum number of tasks to run in parallel",
				Value:       1,
				Destination: &flags.Tasks.Run.Concurrency,
			},
			&cli.StringSliceFlag{
				Name:        "prepend",
				Usage:       "Commands to insert before the task's command list (repeatable)",
				Destination: &flags.Tasks.Run.Prepend,
			},
			&cli.StringSliceFlag{
				Name:        "append",
				Usage:       "Commands to append after the task's command list (repeatable)",
				Destination: &flags.Tasks.Run.Append,
			},
			&cli.IntFlag{
				Name:        "start",
				Usage:       "Skip first N commands (or items for foreach tasks)",
				Value:       0,
				Destination: &flags.Tasks.Run.Start,
			},
			&cli.DurationFlag{
				Name:        "timeout",
				Usage:       "Override task timeout (0 = use task's configured timeout)",
				Value:       0,
				Destination: &flags.Tasks.Run.Timeout,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := core.GetFlags()

			flags.Tasks.Run.IsSet = cmd.IsSet

			if flags.Tasks.Run.Now {
				return runNow(flags, cmd.Args().Slice())
			}

			return runScheduled(flags, cmd.Args().Slice())
		},
	}
}

// applyTaskOverrides mutates a task definition with CLI flag overrides.
// Splices prepend/append into commands, clears agent/mode when root flags
// are explicitly set, and overrides timeout when the local flag is changed.
func applyTaskOverrides(t *task.Task, flags core.Flags) {
	// Splice prepend/append into command list.
	prepend := sanitizeCLICommands(flags.Tasks.Run.Prepend)
	appendCmds := sanitizeCLICommands(flags.Tasks.Run.Append)

	if len(prepend) > 0 || len(appendCmds) > 0 {
		cmds := make([]string, 0, len(prepend)+len(t.Commands)+len(appendCmds))

		cmds = append(cmds, prepend...)
		cmds = append(cmds, t.Commands...)
		cmds = append(cmds, appendCmds...)
		t.Commands = cmds
	}

	// Root flag precedence: CLI wins over task YAML.
	if flags.IsSet("agent") {
		t.Agent = ""
	}

	if flags.IsSet("mode") {
		t.Mode = ""
	}

	// Local timeout override.
	if flags.Tasks.Run.IsSet("timeout") {
		t.Timeout = flags.Tasks.Run.Timeout
	}
}

// TODO(Idelchi): This is a safeguard against sandboxed environments that add \ in front of !.
func sanitizeCLICommands(cmds []string) []string {
	for i, cmd := range cmds {
		cmds[i] = strings.TrimLeft(cmd, `\`)
	}

	return cmds
}

// resolveTasks loads config and returns the tasks matching the given names.
// If no names are given, returns all scheduled tasks.
func resolveTasks(flags core.Flags, names []string) (task.Tasks, error) {
	opts := flags.ConfigOptions()

	opts.ExtraTaskFiles = flags.Tasks.Files

	cfg, _, err := config.New(opts, config.PartTasks)
	if err != nil {
		return nil, err
	}

	if len(names) == 0 {
		return cfg.Tasks.Scheduled(), nil
	}

	selected := make(task.Tasks)

	for _, name := range names {
		t := cfg.Tasks.Get(name)
		if t == nil {
			return nil, fmt.Errorf("task %q not found\nAvailable: %v", name, cfg.Tasks.Names())
		}

		selected[name] = *t
	}

	return selected, nil
}

// runScheduled starts the scheduler daemon for the given tasks (or all scheduled tasks).
func runScheduled(flags core.Flags, names []string) error {
	opts := flags.ConfigOptions()

	opts.ExtraTaskFiles = flags.Tasks.Files

	cfg, _, err := config.New(opts, config.PartTasks)
	if err != nil {
		return err
	}

	scheduled := cfg.Tasks.Scheduled()

	if len(names) > 0 {
		filtered := make(task.Tasks)

		for _, name := range names {
			t, ok := scheduled[name]
			if !ok {
				return fmt.Errorf("task %q is not scheduled (missing schedule or disabled)", name)
			}

			filtered[name] = t
		}

		scheduled = filtered
	}

	if len(scheduled) == 0 {
		fmt.Fprintln(flags.Writer, "No scheduled tasks found.")

		return nil
	}

	for _, name := range scheduled.Names() {
		t := scheduled.Get(name)
		applyTaskOverrides(t, flags)

		scheduled[name] = *t
	}

	fmt.Fprintf(flags.Writer, "Starting scheduler with %d task(s):\n", len(scheduled))

	for _, name := range scheduled.Names() {
		t := scheduled[name]
		fmt.Fprintf(flags.Writer, "  - %s: %s\n", name, t.Schedule)
	}

	// Each task execution creates a fresh RunSession to avoid
	// conversation state leaking between scheduled runs.
	runFn := func(ctx context.Context, t task.Task) error {
		return core.RunSession(
			flags,
			core.HeadlessUI,
			func(sessCtx context.Context, _ context.CancelCauseFunc, asst *assistant.Assistant, u ui.UI) error {
				go u.Run(sessCtx) //nolint:errcheck

				// Merge both contexts: ctx carries the task timeout from the scheduler,
				// sessCtx carries signal cancellation from RunSession. The task must
				// stop when either fires.
				taskCtx, cancel := context.WithCancel(sessCtx)

				go func() {
					select {
					case <-ctx.Done():
						cancel()
					case <-taskCtx.Done():
					}
				}()

				defer cancel()

				return runTask(flags.Writer, taskCtx, asst, u, t, flags.Debug, flags.Workdir, flags.Tasks.Run.Start)
			},
		)
	}

	sched, err := task.NewScheduler(scheduled, flags.Tasks.Run.Concurrency, runFn)
	if err != nil {
		return err
	}

	sched.Start()

	fmt.Fprintln(flags.Writer, "Scheduler started. Press Ctrl+C to stop.")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	fmt.Fprintln(flags.Writer, "\nShutting down scheduler...")

	return sched.Shutdown()
}

// runNow runs tasks immediately with concurrency control and exits.
// With names: runs those specific tasks (regardless of schedule status).
// Without names: runs all scheduled tasks.
func runNow(flags core.Flags, names []string) error {
	tasks, err := resolveTasks(flags, names)
	if err != nil {
		return err
	}

	if len(tasks) == 0 {
		fmt.Fprintln(flags.Writer, "No tasks to run.")

		return nil
	}

	for _, name := range tasks.Names() {
		t := tasks.Get(name)
		applyTaskOverrides(t, flags)

		tasks[name] = *t
	}

	if flags.Dry == "render" {
		for _, name := range tasks.Names() {
			t := tasks[name]
			renderTask(&t, flags)
		}
	}

	concurrency := flags.Tasks.Run.Concurrency
	if concurrency == 0 {
		concurrency = 1
	}

	g := new(errgroup.Group)
	g.SetLimit(int(concurrency))

	for _, name := range tasks.Names() {
		t := tasks[name]

		g.Go(func() error {
			return core.RunSession(
				flags,
				core.HeadlessUI,
				func(sessCtx context.Context, _ context.CancelCauseFunc, asst *assistant.Assistant, u ui.UI) error {
					go u.Run(sessCtx) //nolint:errcheck

					taskCtx := sessCtx

					if t.Timeout > 0 {
						var cancel context.CancelFunc

						taskCtx, cancel = context.WithTimeout(sessCtx, t.Timeout)

						defer cancel()
					}

					return runTask(flags.Writer, taskCtx, asst, u, t, flags.Debug, flags.Workdir, flags.Tasks.Run.Start)
				},
			)
		})
	}

	return g.Wait()
}

// renderTask prints a task's resolved configuration for dry-run inspection.
func renderTask(t *task.Task, flags core.Flags) {
	fmt.Fprintf(flags.Writer, "Task:     %s\n", t.Name)
	fmt.Fprintf(flags.Writer, "Timeout:  %s\n", t.Timeout)

	if t.Agent != "" {
		fmt.Fprintf(flags.Writer, "Agent:    %s\n", t.Agent)
	}

	if t.Mode != "" {
		fmt.Fprintf(flags.Writer, "Mode:     %s\n", t.Mode)
	}

	if t.Session != "" {
		fmt.Fprintf(flags.Writer, "Session:  %s\n", t.Session)
	}

	if t.Workdir != "" {
		fmt.Fprintf(flags.Writer, "Workdir:  %s\n", t.Workdir)
	}

	if len(t.Pre) > 0 {
		fmt.Fprintf(flags.Writer, "Pre:      %v\n", t.Pre)
	}

	if flags.Tasks.Run.Start > 0 {
		fmt.Fprintf(flags.Writer, "Start:    %d\n", flags.Tasks.Run.Start)
	}

	if len(t.Commands) > 0 {
		fmt.Fprintln(flags.Writer, "Commands:")

		for i, c := range t.Commands {
			fmt.Fprintf(flags.Writer, "  [%d] %s\n", i+1, c)
		}
	}

	if len(t.Post) > 0 {
		fmt.Fprintf(flags.Writer, "Post:     %v\n", t.Post)
	}

	if t.ForEach != nil {
		fmt.Fprintf(flags.Writer, "ForEach:  %+v\n", t.ForEach)
	}

	if len(t.Finally) > 0 {
		fmt.Fprintf(flags.Writer, "Finally:  %v\n", t.Finally)
	}

	fmt.Fprintln(flags.Writer)
}
