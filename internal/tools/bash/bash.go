package bash

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"

	"github.com/MakeNowJust/heredoc/v2"
	sprig "github.com/go-task/slim-sprig/v3"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/task"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/godyl/pkg/env"
	"github.com/idelchi/godyl/pkg/path/file"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// DefaultTimeout is the default command timeout.
const DefaultTimeout = 15 * time.Second

// LimitedBuffer is an io.Writer that wraps bytes.Buffer with a byte-level cap.
// When max <= 0, writes pass through without capping.
// When the cap is reached, excess bytes are silently discarded.
type LimitedBuffer struct {
	buf bytes.Buffer
	max int
	hit bool
}

func (lb *LimitedBuffer) Write(p []byte) (int, error) {
	if lb.max <= 0 {
		return lb.buf.Write(p)
	}

	remaining := lb.max - lb.buf.Len()
	if remaining <= 0 {
		lb.hit = true

		return len(p), nil
	}

	if len(p) > remaining {
		lb.hit = true
		p = p[:remaining]
	}

	return lb.buf.Write(p)
}

func (lb *LimitedBuffer) String() string { return lb.buf.String() }
func (lb *LimitedBuffer) Len() int       { return lb.buf.Len() }
func (lb *LimitedBuffer) Capped() bool   { return lb.hit }

// execHandlerMiddleware returns a middleware that properly kills process groups on timeout.
func execHandlerMiddleware(killTimeout time.Duration) func(interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(_ interp.ExecHandlerFunc) interp.ExecHandlerFunc {
		return func(ctx context.Context, args []string) error {
			hc := interp.HandlerCtx(ctx)

			path, err := interp.LookPathDir(hc.Dir, hc.Env, args[0])
			if err != nil {
				fmt.Fprintln(hc.Stderr, err)

				return interp.ExitStatus(127)
			}

			cmd := newCommand(path, args, hc)

			err = cmd.Start()
			if err == nil {
				registerProcess(cmd.Process.Pid)

				stopf := context.AfterFunc(ctx, func() {
					pgid := cmd.Process.Pid

					if killTimeout <= 0 {
						_ = killProcessGroup(pgid, syscall.SIGKILL)

						return
					}

					_ = killProcessGroup(pgid, syscall.SIGTERM)

					time.Sleep(killTimeout)

					_ = killProcessGroup(pgid, syscall.SIGKILL)
				})
				defer stopf()

				err = cmd.Wait()
			}

			return handleExecError(ctx, hc, err)
		}
	}
}

// execEnv converts an expand.Environ to a []string for os/exec.
func execEnv(env expand.Environ) []string {
	var list []string

	env.Each(func(name string, vr expand.Variable) bool {
		if vr.Exported && vr.Kind == expand.String {
			list = append(list, name+"="+vr.String())
		}

		return true
	})

	return list
}

// Inputs defines the parameters for the Bash tool.
type Inputs struct {
	Command   string `json:"command"              jsonschema:"required,description=Shell command or script to execute"      validate:"required"`
	Workdir   string `json:"workdir,omitempty"    jsonschema:"description=Working directory to execute command in"`
	TimeoutMS int64  `json:"timeout_ms,omitempty" jsonschema:"description=Command timeout in milliseconds (default: 15000)"`
}

// Tool implements the Bash command execution tool using mvdan/sh interpreter.
type Tool struct {
	tool.Base

	Timeout    time.Duration
	Truncation config.BashTruncation
	Rewrite    string

	mu        sync.Mutex
	tempFiles []string
}

// New creates a new Bash tool with documentation.
func New(truncation config.BashTruncation, rewrite string) *Tool {
	return &Tool{
		Truncation: truncation,
		Rewrite:    rewrite,
		Base: tool.Base{
			Text: tool.Text{
				Description: `Execute shell commands inside a bash session.`,
				Usage: heredoc.Doc(`
					Use for general shell commands when no dedicated tool fits.
					Avoid using Bash for file reading, writing, or editing.
					NEVER prefix the command with 'bash', 'bash -lc', or anything similar.

					Do not change the default timeout unless absolutely necessary.
					If you have to increase the timeout significantly, you most probably have an issue that you should address differently.

					Output is byte-capped at 1MB per stream (stdout/stderr) to prevent memory issues.
					Output over 200 lines is automatically truncated to the first 100 and last 80 lines.
					The full output is saved to a temp file shown in the truncation message.
					Use Read or Rg on that file to access the complete output.
				`),
				Examples: heredoc.Doc(`
					{"command": "go build ."}
					{"command": "npm run build", "workdir": "frontend"}
					{"command": "go run .", "timeout_ms": 10000}
				`),
			},
		},
	}
}

// Name returns the tool's identifier.
func (t *Tool) Name() string {
	return "Bash"
}

// Close removes any temp files created during truncated output.
func (t *Tool) Close() {
	t.mu.Lock()
	files := t.tempFiles

	t.tempFiles = nil
	t.mu.Unlock()

	for _, f := range files {
		file.New(f).Remove()
	}
}

// Schema returns the provider-agnostic tool definition.
func (t *Tool) Schema() tool.Schema {
	return tool.GenerateSchema[Inputs](t)
}

// Execute runs the bash command using mvdan/sh interpreter.
func (t *Tool) Execute(ctx context.Context, args map[string]any) (string, error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return "", err
	}

	command := params.Command

	if t.Rewrite != "" {
		data := struct{ Command string }{command}

		tmpl, err := template.New("rewrite").Funcs(sprig.FuncMap()).Parse(t.Rewrite)
		if err != nil {
			return "", fmt.Errorf("bash.rewrite template parse: %w", err)
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			return "", fmt.Errorf("bash.rewrite template exec: %w", err)
		}

		command = buf.String()
	}

	parser := syntax.NewParser()

	file, err := parser.Parse(strings.NewReader(command), "")
	if err != nil {
		return "", fmt.Errorf("syntax error: %w", err)
	}

	// Determine timeout
	timeout := t.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	if params.TimeoutMS > 0 {
		timeout = time.Duration(params.TimeoutMS) * time.Millisecond
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Capture stdout/stderr with byte-level cap.
	maxBytes := 0

	if t.Truncation.MaxOutputBytes != nil {
		maxBytes = *t.Truncation.MaxOutputBytes
	}

	stdout := &LimitedBuffer{max: maxBytes}
	stderr := &LimitedBuffer{max: maxBytes}

	// Wrap with streaming writer if a callback is present in context.
	var stdoutWriter, stderrWriter io.Writer = stdout, stderr

	if cb := tool.StreamCallbackFromContext(ctx); cb != nil {
		stdoutWriter = NewStreamingWriter(stdout, cb)
		stderrWriter = NewStreamingWriter(stderr, cb)
	}

	// Build interpreter options
	taskEnv := task.EnvFromContext(ctx)
	environ := taskEnv.MergedWith(env.FromEnv())

	opts := []interp.RunnerOption{
		interp.StdIO(nil, stdoutWriter, stderrWriter),
		interp.Env(expand.ListEnviron(environ.AsSlice()...)),
		interp.ExecHandlers(execHandlerMiddleware(time.Second)),
	}

	// Set working directory: explicit param > context workdir.
	wd := params.Workdir
	if wd == "" {
		wd = tool.WorkDirFromContext(ctx)
	}

	if wd != "" {
		wd = tool.ResolvePath(ctx, os.ExpandEnv(wd))
		opts = append(opts, interp.Dir(wd))
	}

	// Create interpreter
	runner, err := interp.New(opts...)
	if err != nil {
		return "", fmt.Errorf("creating shell runner: %w", err)
	}

	// Execute
	err = runner.Run(execCtx, file)

	// Flush any remaining streaming output.
	if sw, ok := stdoutWriter.(*StreamingWriter); ok {
		sw.Flush()
	}

	if sw, ok := stderrWriter.(*StreamingWriter); ok {
		sw.Flush()
	}

	// Collect output
	output := stdout.String()

	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}

		output += "STDERR:\n" + stderr.String()
	}

	// Append byte-cap marker if either stream was capped.
	if stdout.Capped() || stderr.Capped() {
		output += fmt.Sprintf("\n[output capped at %d byte limit — full output not available]", maxBytes)
	}

	// Handle errors
	if err != nil {
		// timeout
		if execCtx.Err() == context.DeadlineExceeded {
			output += fmt.Sprintf("\nEXIT CODE: TIMEOUT after %v", timeout)

			return t.truncateOutput(output, t.Truncation), nil
		}

		// command exited with a non-zero code (normal situation)
		var exitStatus interp.ExitStatus
		if errors.As(err, &exitStatus) {
			output += fmt.Sprintf("\nEXIT CODE: %d", uint8(exitStatus))

			return t.truncateOutput(output, t.Truncation), nil
		}

		// internal interpreter failure — only real tool error case
		return output, fmt.Errorf("command execution: %w", err)
	}

	return t.truncateOutput(output, t.Truncation), nil
}

// truncateOutput middle-truncates output exceeding cfg.MaxLines.
// Keeps the first HeadLines and last TailLines, saving full output to a temp file.
// nil MaxLines means disabled (no truncation). Zero means exit-code only.
func (t *Tool) truncateOutput(output string, cfg config.BashTruncation) string {
	if cfg.MaxLines == nil {
		return output
	}

	maxLines := *cfg.MaxLines

	lines := strings.Split(output, "\n")
	if len(lines) <= maxLines {
		return output
	}

	headLines, tailLines := 0, 0

	if cfg.HeadLines != nil {
		headLines = *cfg.HeadLines
	}

	if cfg.TailLines != nil {
		tailLines = *cfg.TailLines
	}

	// Clamp to available lines.
	headLines = min(headLines, len(lines))
	tailLines = min(tailLines, len(lines)-headLines)

	// Save full output to temp file.
	filePath := "(temp file creation failed)"

	if tmp, err := file.CreateRandomInDir("", "aura-bash-output-*.txt"); err == nil {
		if writeErr := tmp.Write([]byte(output)); writeErr != nil {
			debug.Log("[bash] temp file write: %v", writeErr)
		} else {
			filePath = tmp.Path()
		}

		t.mu.Lock()
		t.tempFiles = append(t.tempFiles, tmp.Path())
		t.mu.Unlock()
	}

	omitted := len(lines) - headLines - tailLines

	var b strings.Builder

	if headLines > 0 {
		b.WriteString(strings.Join(lines[:headLines], "\n"))
		b.WriteByte('\n')
	}

	fmt.Fprintf(&b, "... (%d lines truncated, full output: %s) ...", omitted, filePath)

	if tailLines > 0 {
		b.WriteByte('\n')
		b.WriteString(strings.Join(lines[len(lines)-tailLines:], "\n"))
	}

	return b.String()
}
