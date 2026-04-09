// Package hooks provides user-configurable shell hooks that run before and after LLM tool calls.
//
// Hooks are defined in .aura/config/hooks/**/*.yaml, matched by tool name regex, and executed via mvdan/sh.
// They receive JSON context on stdin and control behavior through exit codes and optional JSON stdout.
//
// Exit code semantics:
//   - 0: success — parse stdout for optional {"message": "...", "deny": true, "reason": "..."}
//   - 2: block (pre) or feedback (post) — stderr is returned as the message
//   - other: non-blocking — stderr (or stdout) is returned as feedback to the LLM
package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/task"
	"github.com/idelchi/godyl/pkg/dag"
	"github.com/idelchi/godyl/pkg/env"
	"github.com/idelchi/godyl/pkg/path/file"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// DefaultTimeout is the default hook execution timeout.
const DefaultTimeout = 10 * time.Second

// Entry is a compiled hook ready to match and execute.
type Entry struct {
	Name    string         // hook name from config map key
	Matcher *regexp.Regexp // nil = match all tools
	Files   string         // glob pattern for filepath.Match against basenames
	Command string
	Timeout time.Duration
	Silent  bool // suppress all output and exit codes
}

// Runner holds compiled pre and post hooks. Zero value is safe — all methods
// return immediately when no hooks are configured. This is a value type; no
// nil checks needed at call sites.
type Runner struct {
	Pre     []Entry
	Post    []Entry
	OnStart func(hookName string) // called before each matching hook executes; nil = no-op
}

// Event is the JSON payload piped to hook stdin.
type Event struct {
	HookEvent string `json:"hook_event"`
	Tool      struct {
		Name   string `json:"name"`
		Input  any    `json:"input"`
		Output string `json:"output,omitempty"`
	} `json:"tool"`
	CWD       string   `json:"cwd"`
	FilePaths []string `json:"file_paths,omitempty"`
}

// Result is the outcome of running hooks for a single event.
type Result struct {
	Blocked bool
	Message string
}

// hookOutput is the optional JSON a hook may print to stdout on exit 0.
type hookOutput struct {
	Message string `json:"message"`
	Deny    bool   `json:"deny"`
	Reason  string `json:"reason"`
}

// Order splits hooks by event and returns topologically sorted name lists.
// Useful for display without compiling regexes.
func Order(hks config.Hooks) (pre, post []string, err error) {
	preNames := make([]string, 0)
	postNames := make([]string, 0)

	for name, h := range hks {
		if h.IsDisabled() {
			continue
		}

		switch h.Event {
		case "pre":
			preNames = append(preNames, name)
		case "post":
			postNames = append(postNames, name)
		}
	}

	slices.Sort(preNames)
	slices.Sort(postNames)

	pre, err = topoSort(preNames, hks)
	if err != nil {
		return nil, nil, fmt.Errorf("pre hooks: %w", err)
	}

	post, err = topoSort(postNames, hks)
	if err != nil {
		return nil, nil, fmt.Errorf("post hooks: %w", err)
	}

	return pre, post, nil
}

// New compiles hook entries from config and orders them by dependency DAG.
// Panics on invalid regex (fail-fast). Returns error on dependency cycles or missing deps.
func New(hks config.Hooks) (Runner, error) {
	pre, post, err := Order(hks)
	if err != nil {
		return Runner{}, err
	}

	return Runner{
		Pre:  compileEntries(pre, hks),
		Post: compileEntries(post, hks),
	}, nil
}

// topoSort builds a DAG from hook dependencies and returns names in topological order.
func topoSort(names []string, hks config.Hooks) ([]string, error) {
	if len(names) == 0 {
		return nil, nil
	}

	g, err := dag.Build(names, func(n string) []string {
		return hks[n].Depends
	})
	if err != nil {
		return nil, err
	}

	return g.Topo(), nil
}

// compileEntries converts sorted hook names into compiled entries.
func compileEntries(names []string, hks config.Hooks) []Entry {
	entries := make([]Entry, 0, len(names))

	for _, name := range names {
		entries = append(entries, compileHook(name, hks[name]))
	}

	return entries
}

// RunPre runs matching pre-hooks sequentially. Returns on first block.
func (r Runner) RunPre(ctx context.Context, toolName string, toolInput any, cwd string) Result {
	if len(r.Pre) == 0 {
		return Result{}
	}

	event := Event{
		HookEvent: "PreToolUse",
		CWD:       cwd,
		FilePaths: extractPaths(toolInput),
	}

	event.Tool.Name = toolName
	event.Tool.Input = toolInput

	return r.runAll(ctx, r.Pre, event, true)
}

// RunPost runs matching post-hooks sequentially. Collects messages.
func (r Runner) RunPost(ctx context.Context, toolName string, toolInput any, toolOutput, cwd string) Result {
	if len(r.Post) == 0 {
		return Result{}
	}

	event := Event{
		HookEvent: "PostToolUse",
		CWD:       cwd,
		FilePaths: extractPaths(toolInput),
	}

	event.Tool.Name = toolName
	event.Tool.Input = toolInput
	event.Tool.Output = toolOutput

	return r.runAll(ctx, r.Post, event, false)
}

// runAll executes all matching hooks sequentially. If canBlock is true, the first
// hook that blocks (exit 2 or JSON deny) short-circuits and returns immediately.
func (r Runner) runAll(ctx context.Context, entries []Entry, event Event, canBlock bool) Result {
	eventJSON, err := json.Marshal(event)
	if err != nil {
		debug.Log("[hooks] failed to marshal event: %v", err)

		return Result{}
	}

	var messages []string

	for _, entry := range entries {
		if !entry.Matches(event.Tool.Name) {
			continue
		}

		// Filter by file glob if set
		matched := entry.MatchedPaths(event.FilePaths)
		if entry.Files != "" && len(matched) == 0 {
			continue
		}

		if r.OnStart != nil {
			r.OnStart(entry.Name)
		}

		result := r.execute(ctx, entry, string(eventJSON), matched)

		if result.Blocked && canBlock {
			debug.Log("[hooks] %s blocked by %s: %s → %s", event.Tool.Name, entry.Name, entry.Command, result.Message)

			result.Message = fmt.Sprintf("[Hook: %s]:\n%s", entry.Name, result.Message)

			return result
		} else if result.Blocked {
			debug.Log(
				"[hooks] post-hook %s returned deny but post-hooks cannot block (tool already executed)",
				entry.Name,
			)
		}

		if result.Message != "" {
			messages = append(messages, fmt.Sprintf("[Hook: %s]:\n%s", entry.Name, result.Message))
		}
	}

	if len(messages) > 0 {
		return Result{Message: strings.Join(messages, "\n")}
	}

	return Result{}
}

// execute runs a single hook command and interprets exit code + output.
func (r Runner) execute(ctx context.Context, entry Entry, eventJSON string, filePaths []string) Result {
	ctx, cancel := context.WithTimeout(ctx, entry.Timeout)
	defer cancel()

	prog, err := syntax.NewParser().Parse(strings.NewReader(entry.Command), entry.Name)
	if err != nil {
		debug.Log("[hooks] syntax error in %s: %v", entry.Name, err)

		return Result{Message: fmt.Sprintf("hook error: syntax error in %s: %v", entry.Name, err)}
	}

	taskEnv := task.EnvFromContext(ctx)
	e := taskEnv.MergedWith(env.FromEnv())

	if len(filePaths) > 0 {
		e.AddPair("FILE", strings.Join(filePaths, " "))
	}

	if entry.Silent {
		runner, err := interp.New(
			interp.StdIO(strings.NewReader(eventJSON), io.Discard, io.Discard),
			interp.Env(expand.ListEnviron(e.AsSlice()...)),
		)
		if err != nil {
			debug.Log("[hooks] runner error: %v", err)

			return Result{Message: fmt.Sprintf("hook error: runner init failed for %s: %v", entry.Name, err)}
		}

		debug.Log("[hooks] running %s (silent): %s", entry.Name, entry.Command)

		if err := runner.Run(ctx, prog); err != nil {
			debug.Log("[hooks] %s (silent) failed: %v", entry.Name, err)
		}

		return Result{}
	}

	var stdout, stderr bytes.Buffer

	runner, err := interp.New(
		interp.StdIO(strings.NewReader(eventJSON), &stdout, &stderr),
		interp.Env(expand.ListEnviron(e.AsSlice()...)),
	)
	if err != nil {
		debug.Log("[hooks] runner error: %v", err)

		return Result{Message: fmt.Sprintf("hook error: runner init failed for %s: %v", entry.Name, err)}
	}

	debug.Log("[hooks] running %s: %s", entry.Name, entry.Command)

	err = runner.Run(ctx, prog)

	exitCode := 0

	if err != nil {
		var exitStatus interp.ExitStatus
		if errors.As(err, &exitStatus) {
			exitCode = int(exitStatus)
		} else if ctx.Err() != nil {
			if ctx.Err() == context.DeadlineExceeded {
				// Hook's own timeout expired — surface to user.
				debug.Log("[hooks] %s timed out after %s: %v", entry.Name, entry.Timeout, err)

				return Result{Message: fmt.Sprintf("hook %q timed out after %s", entry.Name, entry.Timeout)}
			}

			// User cancellation (Ctrl+C) — silent, debug-only.
			debug.Log("[hooks] %s cancelled: %v", entry.Name, err)

			return Result{}
		} else {
			// Genuine interpreter failure.
			debug.Log("[hooks] exec error: %v", err)

			return Result{Message: fmt.Sprintf("hook error: %s execution failed: %v", entry.Name, err)}
		}
	}

	switch exitCode {
	case 0:
		return r.parseSuccessOutput(stdout.String())
	case 2:
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = "hook blocked: " + entry.Command
		}

		return Result{Blocked: true, Message: msg}
	default:
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}

		debug.Log("[hooks] exit %d from %s: %s", exitCode, entry.Command, msg)

		return Result{Message: msg}
	}
}

// parseSuccessOutput parses optional JSON from stdout on exit 0.
func (r Runner) parseSuccessOutput(stdout string) Result {
	stdout = strings.TrimSpace(stdout)
	if stdout == "" {
		return Result{}
	}

	var out hookOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		// Not JSON — treat as plain text message.
		return Result{Message: stdout}
	}

	if out.Deny {
		msg := out.Reason
		if msg == "" {
			msg = "denied by hook"
		}

		return Result{Blocked: true, Message: msg}
	}

	return Result{Message: out.Message}
}

// Matches returns true if the entry's matcher regex matches the tool name (or matcher is nil = match all).
func (e Entry) Matches(toolName string) bool {
	if e.Matcher == nil {
		return true
	}

	return e.Matcher.MatchString(toolName)
}

// compileHook converts a config hook into a compiled entry.
func compileHook(name string, h config.Hook) Entry {
	timeout := DefaultTimeout

	if h.TimeoutSeconds() > 0 {
		timeout = time.Duration(h.TimeoutSeconds()) * time.Second
	}

	var matcher *regexp.Regexp

	if h.Matcher != "" {
		matcher = regexp.MustCompile(h.Matcher)
	}

	return Entry{
		Name:    name,
		Matcher: matcher,
		Files:   h.Files,
		Command: h.Command,
		Timeout: timeout,
		Silent:  h.IsSilent(),
	}
}

// MatchedPaths returns file paths that match the entry's files glob.
// If no files glob is set, returns all paths unchanged.
func (e Entry) MatchedPaths(filePaths []string) []string {
	if e.Files == "" {
		return filePaths
	}

	var matched []string

	for _, p := range filePaths {
		if ok, _ := file.New(file.New(p).Base()).Matches(e.Files); ok {
			matched = append(matched, p)
		}
	}

	return matched
}

// extractPaths pulls file paths from tool input arguments.
func extractPaths(toolInput any) []string {
	args, ok := toolInput.(map[string]any)
	if !ok {
		return nil
	}

	// Write, Read, etc. — use "path" field
	if p, ok := args["path"].(string); ok && p != "" {
		return []string{p}
	}

	// Patch — file paths embedded in patch content
	if patch, ok := args["patch"].(string); ok && patch != "" {
		return extractPatchPaths(patch)
	}

	return nil
}

// extractPatchPaths parses file paths from patch header lines.
// Matches: "*** Add File: <path>", "*** Update File: <path>", "*** Delete File: <path>".
func extractPatchPaths(patch string) []string {
	prefixes := []string{
		"*** Add File: ",
		"*** Update File: ",
		"*** Delete File: ",
	}

	var paths []string

	for line := range strings.SplitSeq(patch, "\n") {
		line = strings.TrimSpace(line)

		for _, prefix := range prefixes {
			if after, ok := strings.CutPrefix(line, prefix); ok {
				if p := strings.TrimSpace(after); p != "" {
					paths = append(paths, p)
				}
			}
		}
	}

	return paths
}
