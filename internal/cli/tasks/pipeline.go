package tasks

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/idelchi/aura/internal/assistant"
	"github.com/idelchi/aura/internal/cli/core"
	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/session"
	"github.com/idelchi/aura/internal/task"
	"github.com/idelchi/aura/internal/tmpl"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/godyl/pkg/env"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/folder"

	"go.yaml.in/yaml/v4"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// execShell runs a single shell command via mvdan/sh, printing output to stdout/stderr.
// Returns error on non-zero exit (interp.ExitStatus) or interpreter failure.
func execShell(ctx context.Context, command string, extraEnv map[string]string) error {
	prog, err := syntax.NewParser().Parse(strings.NewReader(command), "hook")
	if err != nil {
		return err
	}

	e := env.Env(extraEnv)
	environ := e.MergedWith(env.FromEnv())

	runner, err := interp.New(
		interp.StdIO(nil, os.Stdout, os.Stderr),
		interp.Env(expand.ListEnviron(environ.AsSlice()...)),
	)
	if err != nil {
		return err
	}

	return runner.Run(ctx, prog)
}

// captureShell runs a shell command and returns its stdout as bytes.
func captureShell(ctx context.Context, command string, extraEnv map[string]string) ([]byte, error) {
	prog, err := syntax.NewParser().Parse(strings.NewReader(command), "foreach")
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer

	e := env.Env(extraEnv)
	environ := e.MergedWith(env.FromEnv())

	runner, err := interp.New(
		interp.StdIO(nil, &buf, os.Stderr),
		interp.Env(expand.ListEnviron(environ.AsSlice()...)),
	)
	if err != nil {
		return nil, err
	}

	if err := runner.Run(ctx, prog); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// runHooks executes shell commands sequentially, aborting on first failure.
// Each command is template-expanded with vars before shell execution.
func runHooks(
	w io.Writer,
	ctx context.Context,
	taskName, phase string,
	commands []string,
	verbose bool,
	vars map[string]string,
	extraEnv map[string]string,
) error {
	for i, cmd := range commands {
		if ctx.Err() != nil {
			return fmt.Errorf("task %q %s cancelled at hook %d/%d", taskName, phase, i+1, len(commands))
		}

		expanded, err := expandCommand(cmd, vars)
		if err != nil {
			return fmt.Errorf("task %q %s hook %d: expanding template: %w", taskName, phase, i+1, err)
		}

		if verbose {
			fmt.Fprintf(w, "[task:%s] [%s %d/%d] %s\n", taskName, phase, i+1, len(commands), expanded)
		}

		if err := execShell(ctx, expanded, extraEnv); err != nil {
			return fmt.Errorf("task %q %s hook %d (%s): %w", taskName, phase, i+1, expanded, err)
		}
	}

	return nil
}

// resolveItems reads iteration items from a foreach source.
// Relative file paths are resolved against the config home directory.
func resolveItems(
	ctx context.Context,
	fe *task.ForEach,
	configHome string,
	extraEnv map[string]string,
) ([]string, error) {
	var raw []byte

	if fe.File != "" {
		f := file.New(fe.File)
		if !f.IsAbs() {
			f = folder.New(configHome).WithFile(fe.File)
		}

		data, err := f.Read()
		if err != nil {
			return nil, fmt.Errorf("reading foreach file %q: %w", fe.File, err)
		}

		raw = data
	} else {
		data, err := captureShell(ctx, fe.Shell, extraEnv)
		if err != nil {
			return nil, fmt.Errorf("running foreach shell %q: %w", fe.Shell, err)
		}

		raw = data
	}

	lines := strings.Split(string(raw), "\n")

	items := lines[:0]
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			items = append(items, trimmed)
		}
	}

	return items, nil
}

// resolveTaskEnv loads env_file entries and expands env values with runtime templates.
// Precedence: env > env_file > process env (process env is implicit via os.Environ at exec time).
func resolveTaskEnv(
	envFiles []string,
	rawEnv map[string]string,
	configHome string,
	baseVars map[string]string,
) (env.Env, error) {
	if len(envFiles) == 0 && len(rawEnv) == 0 {
		return nil, nil
	}

	result := make(env.Env)

	for _, p := range envFiles {
		ef := file.New(p)
		if !ef.IsAbs() {
			ef = folder.New(configHome).WithFile(p)
		}

		fileEnv, err := env.FromDotEnv(ef.Path())
		if err != nil {
			return nil, fmt.Errorf("loading env file %q: %w", p, err)
		}

		maps.Copy(result, fileEnv)
	}

	for k, v := range rawEnv {
		expanded, err := expandCommand(v, baseVars)
		if err != nil {
			return nil, fmt.Errorf("expanding env %q: %w", k, err)
		}

		result[k] = expanded
	}

	return result, nil
}

// expandCommand expands Go template expressions in a command string with foreach vars.
func expandCommand(cmd string, vars map[string]string) (string, error) {
	result, err := tmpl.Expand([]byte(cmd), vars)
	if err != nil {
		return "", err
	}

	return string(result), nil
}

// runCommands executes a list of commands through the assistant, flushing events between each.
func runCommands(
	w io.Writer,
	ctx context.Context,
	asst *assistant.Assistant,
	u ui.UI,
	taskName string,
	commands []string,
	verbose bool,
	extraEnv map[string]string,
) error {
	for i, cmd := range commands {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if verbose {
			fmt.Fprintf(w, "[task:%s] [%d/%d] %s\n", taskName, i+1, len(commands), cmd)
		}

		// Shell command: strip "!" prefix and run via execShell instead of the LLM.
		trimmed := strings.TrimSpace(cmd)
		if shell, ok := strings.CutPrefix(trimmed, "!"); ok {
			shell = strings.TrimSpace(shell)
			if shell == "" {
				return fmt.Errorf("task %q command %d: empty shell command after '!'", taskName, i+1)
			}

			if err := execShell(ctx, shell, extraEnv); err != nil {
				return fmt.Errorf("task %q command %d (%s): %w", taskName, i+1, cmd, err)
			}

			continue
		}

		if err := asst.ProcessInput(ctx, cmd); err != nil {
			return fmt.Errorf("task %q command %d (%s): %w", taskName, i+1, cmd, err)
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

	return nil
}

// runTask executes a single task's commands through an assistant.
// When foreach is set, commands are expanded and executed per-item in a loop.
// Verbose controls whether progress lines like [task:name] are printed.
// cliWorkdir is the value of --workdir flag; when non-empty it takes precedence over task.Workdir.
// start skips the first N commands (no foreach) or N items (foreach).
func runTask(
	w io.Writer,
	ctx context.Context,
	asst *assistant.Assistant,
	u ui.UI,
	t task.Task,
	verbose bool,
	cliWorkdir string,
	start int,
) error {
	// Resume or prepare named session
	if t.Session != "" {
		resolved, err := asst.SessionManager().Find(t.Session)

		switch {
		case errors.Is(err, session.ErrNotFound):
			// First run — set eager name, auto-save will create it
			if setErr := asst.SessionManager().SetName(t.Session); setErr != nil {
				return fmt.Errorf("task %q: setting session name %q: %w", t.Name, t.Session, setErr)
			}
		case err != nil:
			return fmt.Errorf("task %q: resolving session %q: %w", t.Name, t.Session, err)
		default:
			sess, err := asst.SessionManager().Resume(resolved)
			if err != nil {
				return fmt.Errorf("task %q: resuming session %q: %w", t.Name, t.Session, err)
			}

			warnings := asst.ResumeSession(ctx, sess)

			if verbose {
				for _, warning := range warnings {
					fmt.Fprintf(w, "[task:%s] session warning: %s\n", t.Name, warning)
				}
			}
		}
	}

	// Agent/mode/tools/features setup runs before task workdir override —
	// agent file paths (e.g. files: [.aura/config/...]) resolve against project root.
	if t.Agent != "" {
		if err := asst.SwitchAgent(t.Agent, "task"); err != nil {
			return fmt.Errorf("task %q: setting agent %q: %w", t.Name, t.Agent, err)
		}
	}

	if t.Mode != "" {
		if err := asst.SwitchMode(t.Mode); err != nil {
			return fmt.Errorf("task %q: setting mode %q: %w", t.Name, t.Mode, err)
		}
	}

	if t.Tools.IsSet() {
		if err := asst.FilterTools(t.Tools); err != nil {
			return fmt.Errorf("task %q: setting tools filter: %w", t.Name, err)
		}
	}

	if t.Features.Kind != 0 {
		var taskFeatures config.Features
		if err := t.Features.Load(&taskFeatures, yaml.WithKnownFields()); err != nil {
			return fmt.Errorf("task %q: decoding features: %w", t.Name, err)
		}

		if err := asst.MergeFeatures(taskFeatures); err != nil {
			return fmt.Errorf("task %q: merging features: %w", t.Name, err)
		}
	}

	// Emit status after all task overrides (agent, mode, tools, features) are applied.
	u.Events() <- ui.StatusChanged{Status: asst.Status()}

	u.Events() <- ui.DisplayHintsChanged{Hints: asst.DisplayHints()}

	// Resolve effective working directory AFTER agent setup.
	// Precedence: --workdir flag > task.workdir > WorkDir.
	effectiveWorkdir := core.WorkDir

	if t.Workdir != "" && cliWorkdir == "" {
		taskWd := folder.New(t.Workdir)
		if !taskWd.IsAbs() {
			taskWd = folder.New(effectiveWorkdir, t.Workdir)
		}

		if !taskWd.Exists() {
			return fmt.Errorf("task %q workdir %q: not a directory or does not exist", t.Name, taskWd)
		}

		if err := asst.SetWorkDir(taskWd.Path()); err != nil {
			return fmt.Errorf("task %q: setting workdir: %w", t.Name, err)
		}

		effectiveWorkdir = taskWd.Path()
	}

	// Base template variables: task vars (defaults) → --set vars (CLI override).
	baseVars := maps.Clone(t.Vars)
	if baseVars == nil {
		baseVars = make(map[string]string)
	}

	maps.Copy(baseVars, asst.TemplateVars()) // --set overrides task vars

	baseVars["Workdir"] = effectiveWorkdir
	baseVars["LaunchDir"] = core.LaunchDir
	baseVars["Date"] = time.Now().Format("2006-01-02-15-04")

	// Resolve task-scoped environment variables.
	taskEnv, err := resolveTaskEnv(t.EnvFile, t.Env, asst.Paths().Home, baseVars)
	if err != nil {
		return fmt.Errorf("task %q: %w", t.Name, err)
	}

	if len(taskEnv) > 0 {
		ctx = task.WithEnv(ctx, taskEnv)
	}

	// Merge expanded env into runtime template vars.
	maps.Copy(baseVars, taskEnv)

	// Pre hooks — abort everything on failure.
	if err := runHooks(w, ctx, t.Name, "pre", t.Pre, verbose, baseVars, taskEnv); err != nil {
		return err
	}

	// Foreach loop: resolve items, expand and run commands per item.
	if t.ForEach != nil {
		// Template-expand foreach sources before resolving items.
		if t.ForEach.File != "" {
			expanded, err := expandCommand(t.ForEach.File, baseVars)
			if err != nil {
				return fmt.Errorf("task %q: expanding foreach.file: %w", t.Name, err)
			}

			t.ForEach.File = expanded
		}

		if t.ForEach.Shell != "" {
			expanded, err := expandCommand(t.ForEach.Shell, baseVars)
			if err != nil {
				return fmt.Errorf("task %q: expanding foreach.shell: %w", t.Name, err)
			}

			t.ForEach.Shell = expanded
		}

		items, err := resolveItems(ctx, t.ForEach, asst.Paths().Home, taskEnv)
		if err != nil {
			return fmt.Errorf("task %q: %w", t.Name, err)
		}

		if start > 0 {
			if start >= len(items) {
				return fmt.Errorf("task %q: --start %d out of range (%d items, valid: 0-%d)",
					t.Name, start, len(items), len(items)-1)
			}

			items = items[start:]
		}

		total := strconv.Itoa(len(items))

		if verbose {
			fmt.Fprintf(w, "[task:%s] foreach: %d items, %d commands per item (timeout: %s)\n",
				t.Name, len(items), len(t.Commands), t.Timeout)
		}

		// Local signal handler for SIGINT during fresh-context mode.
		// When continue_on_error is true and the parent context is cancelled
		// programmatically, fresh per-item contexts are created from Background().
		// Those fresh contexts can't detect a late SIGINT through the parent chain,
		// so this channel provides an independent abort signal.
		var abortCh <-chan struct{}

		if t.ForEach.ContinueOnError {
			ch := make(chan struct{})

			abortCh = ch

			localSigCh := make(chan os.Signal, 1)
			sigDone := make(chan struct{})

			signal.Notify(localSigCh, os.Interrupt, syscall.SIGTERM)

			go func() {
				select {
				case <-localSigCh:
					close(ch)
				case <-sigDone:
				}

				signal.Stop(localSigCh)
			}()

			defer close(sigDone)
		}

		var itemErrors []error

		for i, item := range items {
			// Check local abort channel first (catches late SIGINTs in fresh-context mode).
			if abortCh != nil {
				select {
				case <-abortCh:
					return fmt.Errorf("task %q interrupted at item %d/%d", t.Name, i+1, len(items))
				default:
				}
			}

			// Cause-aware context guard: SIGINT = hard-abort, programmatic = fall through
			// when continue_on_error is set, creating a fresh per-item context below.
			if ctx.Err() != nil {
				if errors.Is(context.Cause(ctx), core.ErrUserAbort) || !t.ForEach.ContinueOnError {
					return fmt.Errorf("task %q cancelled at item %d/%d", t.Name, i+1, len(items))
				}
			}

			// Build per-item context: fresh from Background() when parent is cancelled,
			// otherwise inherit the parent context.
			var (
				itemCtx    context.Context
				itemCancel context.CancelFunc
			)

			if ctx.Err() != nil {
				itemCtx, itemCancel = context.WithTimeout(context.Background(), t.Timeout)

				if len(taskEnv) > 0 {
					itemCtx = task.WithEnv(itemCtx, taskEnv)
				}
			} else {
				itemCtx = ctx
				itemCancel = func() {}
			}

			vars := maps.Clone(baseVars)

			vars["Item"] = item
			vars["Index"] = strconv.Itoa(i)
			vars["Total"] = total

			if verbose {
				fmt.Fprintf(w, "[task:%s] [item %d/%d] %s\n", t.Name, i+1, len(items), item)
			}

			expanded := make([]string, 0, len(t.Commands))
			for _, raw := range t.Commands {
				cmd, err := expandCommand(raw, vars)
				if err != nil {
					itemCancel()

					return fmt.Errorf("task %q: expanding command for item %q: %w", t.Name, item, err)
				}

				expanded = append(expanded, cmd)
			}

			maxAttempts := 1

			if t.ForEach.Retries > 0 {
				maxAttempts = t.ForEach.Retries + 1
			}

			var lastErr error

			for attempt := range maxAttempts {
				if attempt > 0 {
					if itemCtx.Err() != nil {
						break
					}

					if verbose {
						fmt.Fprintf(w, "[task:%s] [item %d/%d] retry %d/%d\n",
							t.Name, i+1, len(items), attempt, t.ForEach.Retries)
					}
				}

				lastErr = runCommands(w, itemCtx, asst, u, t.Name, expanded, verbose, taskEnv)
				if lastErr == nil {
					break
				}

				if errors.Is(lastErr, assistant.ErrMaxSteps) && len(t.OnMaxSteps) > 0 {
					if err := runHooks(
						w,
						itemCtx,
						t.Name,
						"on_max_steps",
						t.OnMaxSteps,
						verbose,
						vars,
						taskEnv,
					); err != nil {
						debug.Log("[tasks] on_max_steps hook error for %s: %v", t.Name, err)
					}
				}
			}

			itemCancel()

			if lastErr != nil {
				if t.ForEach.ContinueOnError {
					fmt.Fprintf(w, "[task:%s] [item %d/%d] error (after %d attempts): %v\n",
						t.Name, i+1, len(items), maxAttempts, lastErr)

					itemErrors = append(itemErrors, fmt.Errorf("item %d (%s): %w", i, item, lastErr))

					continue
				}

				return lastErr
			}
		}

		// Determine whether to skip finally (SIGINT = skip, programmatic = run).
		skipFinally := false

		if abortCh != nil {
			select {
			case <-abortCh:
				skipFinally = true
			default:
			}
		}

		// Finally commands run once after the loop.
		if len(t.Finally) > 0 && !skipFinally {
			if verbose {
				fmt.Fprintf(w, "[task:%s] finally: %d commands\n", t.Name, len(t.Finally))
			}

			expanded := make([]string, 0, len(t.Finally))
			for _, raw := range t.Finally {
				cmd, err := expandCommand(raw, baseVars)
				if err != nil {
					return fmt.Errorf("task %q: expanding finally command: %w", t.Name, err)
				}

				expanded = append(expanded, cmd)
			}

			var (
				finallyCtx    context.Context
				finallyCancel context.CancelFunc
			)

			if ctx.Err() != nil {
				finallyCtx, finallyCancel = context.WithTimeout(context.Background(), t.Timeout)

				if len(taskEnv) > 0 {
					finallyCtx = task.WithEnv(finallyCtx, taskEnv)
				}
			} else {
				finallyCtx = ctx
				finallyCancel = func() {}
			}

			err := runCommands(w, finallyCtx, asst, u, t.Name, expanded, verbose, taskEnv)

			finallyCancel()

			if err != nil {
				return err
			}
		}

		if len(itemErrors) > 0 {
			return fmt.Errorf("task %q: %d/%d items failed", t.Name, len(itemErrors), len(items))
		}
	} else {
		// No foreach — expand and run commands once.
		expanded := make([]string, 0, len(t.Commands))
		for _, cmd := range t.Commands {
			result, err := expandCommand(cmd, baseVars)
			if err != nil {
				return fmt.Errorf("task %q: expanding command: %w", t.Name, err)
			}

			expanded = append(expanded, result)
		}

		if start > 0 {
			if start >= len(expanded) {
				return fmt.Errorf("task %q: --start %d out of range (%d commands, valid: 0-%d)",
					t.Name, start, len(expanded), len(expanded)-1)
			}

			expanded = expanded[start:]
		}

		if verbose {
			fmt.Fprintf(w, "[task:%s] executing %d commands (timeout: %s)\n", t.Name, len(expanded), t.Timeout)
		}

		if err := runCommands(w, ctx, asst, u, t.Name, expanded, verbose, taskEnv); err != nil {
			if errors.Is(err, assistant.ErrMaxSteps) && len(t.OnMaxSteps) > 0 {
				if hookErr := runHooks(
					w,
					ctx,
					t.Name,
					"on_max_steps",
					t.OnMaxSteps,
					verbose,
					baseVars,
					taskEnv,
				); hookErr != nil {
					debug.Log("[tasks] on_max_steps hook error for %s: %v", t.Name, hookErr)
				}
			}

			return err
		}
	}

	// Post hooks — commands already ran, but report errors.
	return runHooks(w, ctx, t.Name, "post", t.Post, verbose, baseVars, taskEnv)
}
