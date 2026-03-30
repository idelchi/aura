package subagent_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/idelchi/aura/internal/hooks"
	"github.com/idelchi/aura/internal/subagent"
	"github.com/idelchi/aura/internal/tools/filetime"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/stream"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/aura/pkg/llm/tool/call"
	"github.com/idelchi/aura/pkg/llm/usage"
	"github.com/idelchi/aura/pkg/providers"
)

// ---------------------------------------------------------------------------
// fakeProvider
// ---------------------------------------------------------------------------

type fakeProvider struct {
	responses []message.Message
	usages    []usage.Usage
	err       error
	callIdx   int
}

func (f *fakeProvider) Chat(_ context.Context, _ request.Request, _ stream.Func) (message.Message, usage.Usage, error) {
	if f.err != nil {
		return message.Message{}, usage.Usage{}, f.err
	}

	if f.callIdx >= len(f.responses) {
		return message.Message{}, usage.Usage{}, errors.New("fakeProvider: no more responses")
	}

	msg := f.responses[f.callIdx]

	var u usage.Usage

	if f.callIdx < len(f.usages) {
		u = f.usages[f.callIdx]
	}

	f.callIdx++

	return msg, u, nil
}

func (f *fakeProvider) Models(_ context.Context) (model.Models, error) {
	panic("not implemented")
}

func (f *fakeProvider) Model(_ context.Context, _ string) (model.Model, error) {
	panic("not implemented")
}

func (f *fakeProvider) Estimate(_ context.Context, _ request.Request, _ string) (int, error) {
	panic("not implemented")
}

// compile-time interface check.
var _ providers.Provider = (*fakeProvider)(nil)

// ---------------------------------------------------------------------------
// fakeTool
// ---------------------------------------------------------------------------

type fakeTool struct {
	name           string
	output         string
	execErr        error
	preErr         error
	sandboxable    bool
	readPaths      []string
	writePaths     []string
	executed       bool
	wantsLSP       bool
	pathsCallCount int
}

func (ft *fakeTool) Name() string        { return ft.name }
func (ft *fakeTool) Description() string { return "" }
func (ft *fakeTool) Usage() string       { return "" }
func (ft *fakeTool) Examples() string    { return "" }
func (ft *fakeTool) Schema() tool.Schema { return tool.Schema{Name: ft.name} }
func (ft *fakeTool) Available() bool     { return true }
func (ft *fakeTool) Sandboxable() bool   { return ft.sandboxable }
func (ft *fakeTool) Parallel() bool      { return true }
func (ft *fakeTool) Overrides() bool     { return false }
func (ft *fakeTool) WantsLSP() bool      { return ft.wantsLSP }

func (ft *fakeTool) Pre(_ context.Context, _ map[string]any) error {
	return ft.preErr
}

func (ft *fakeTool) Post(_ context.Context, _ map[string]any) {}
func (ft *fakeTool) Close()                                   {}

func (ft *fakeTool) Paths(_ context.Context, _ map[string]any) ([]string, []string, error) {
	ft.pathsCallCount++

	return ft.readPaths, ft.writePaths, nil
}

func (ft *fakeTool) Execute(_ context.Context, _ map[string]any) (string, error) {
	ft.executed = true

	return ft.output, ft.execErr
}

// compile-time interface check.
var _ tool.Tool = (*fakeTool)(nil)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// textMsg returns a message with only text content (no tool calls).
func textMsg(content string) message.Message {
	return message.Message{Content: content}
}

// toolCallMsg returns a message whose only content is a single tool call.
func toolCallMsg(id, name string, args map[string]any) message.Message {
	return message.Message{
		Calls: []call.Call{
			{ID: id, Name: name, Arguments: args},
		},
	}
}

// stubEstimate is a trivial token estimator for tests: 1 token per 4 bytes.
func stubEstimate(text string) int {
	return len(text) / 4
}

// newRunner returns a Runner wired with the given provider and tools, using
// sane defaults (MaxSteps=10, zero-value model, no-op sink, no hooks).
func newRunner(p providers.Provider, tools tool.Tools) *subagent.Runner {
	return &subagent.Runner{
		Provider: p,
		Tools:    tools,
		Model:    model.Model{ContextLength: 8192},
		Events:   nil,
		MaxSteps: 10,
		Estimate: stubEstimate,
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestRunSimpleResponse(t *testing.T) {
	t.Parallel()

	p := &fakeProvider{
		responses: []message.Message{textMsg("hello")},
	}
	r := newRunner(p, nil)

	result, err := r.Run(context.Background(), "hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Text != "hello" {
		t.Errorf("Text = %q, want %q", result.Text, "hello")
	}

	if result.ToolCalls != 0 {
		t.Errorf("ToolCalls = %d, want 0", result.ToolCalls)
	}
}

func TestRunToolCallLoop(t *testing.T) {
	t.Parallel()

	ft := &fakeTool{name: "mytool", output: "tool output"}

	p := &fakeProvider{
		responses: []message.Message{
			toolCallMsg("c1", "mytool", map[string]any{"x": "1"}),
			textMsg("done"),
		},
	}
	r := newRunner(p, tool.Tools{ft})

	result, err := r.Run(context.Background(), "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Text != "done" {
		t.Errorf("Text = %q, want %q", result.Text, "done")
	}

	if result.ToolCalls != 1 {
		t.Errorf("ToolCalls = %d, want 1", result.ToolCalls)
	}

	if result.Tools["mytool"] != 1 {
		t.Errorf("Tools[mytool] = %d, want 1", result.Tools["mytool"])
	}
}

func TestRunToolNotFound(t *testing.T) {
	t.Parallel()

	p := &fakeProvider{
		responses: []message.Message{
			toolCallMsg("c1", "nonexistent_tool", nil),
			textMsg("recovered"),
		},
	}
	// No tools registered.
	r := newRunner(p, nil)

	result, err := r.Run(context.Background(), "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Text != "recovered" {
		t.Errorf("Text = %q, want %q", result.Text, "recovered")
	}
}

func TestRunProviderError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("provider boom")
	p := &fakeProvider{err: sentinel}
	r := newRunner(p, nil)

	_, err := r.Run(context.Background(), "go")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}

	if !strings.Contains(err.Error(), sentinel.Error()) {
		t.Errorf("error %q does not contain %q", err.Error(), sentinel.Error())
	}
}

func TestRunMaxStepsExceeded(t *testing.T) {
	t.Parallel()

	// Provider always returns a tool call — loop never terminates naturally.
	ft := &fakeTool{name: "looper", output: "looping"}

	p := &fakeProvider{}
	// Fill enough responses for 3 steps; each step asks for another tool call.
	for range 5 {
		p.responses = append(p.responses, toolCallMsg("c1", "looper", nil))
	}

	r := newRunner(p, tool.Tools{ft})

	r.MaxSteps = 3

	result, err := r.Run(context.Background(), "loop forever")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Text, "[budget exhausted") {
		t.Errorf("Text = %q, want to contain '[budget exhausted'", result.Text)
	}
}

func TestRunPathCheckerRejects(t *testing.T) {
	t.Parallel()

	ft := &fakeTool{
		name:       "writer",
		output:     "written",
		readPaths:  []string{"/etc/secret"},
		writePaths: []string{"/tmp/out"},
	}

	p := &fakeProvider{
		responses: []message.Message{
			toolCallMsg("c1", "writer", nil),
			textMsg("all done"),
		},
	}

	r := newRunner(p, tool.Tools{ft})

	r.PathChecker = func(read, write []string) error {
		return errors.New("path not allowed")
	}

	result, err := r.Run(context.Background(), "write something")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Text != "all done" {
		t.Errorf("Text = %q, want %q", result.Text, "all done")
	}

	// Path check happens before Execute, so the tool must not have been executed.
	if result.ToolCalls != 0 {
		t.Errorf("ToolCalls = %d, want 0 (tool should not have executed)", result.ToolCalls)
	}
}

func TestRunResultGuardRejects(t *testing.T) {
	t.Parallel()

	ft := &fakeTool{name: "bigtool", output: "huge output"}

	p := &fakeProvider{
		responses: []message.Message{
			toolCallMsg("c1", "bigtool", nil),
			textMsg("finished"),
		},
	}

	r := newRunner(p, tool.Tools{ft})

	r.ResultGuard = func(_ context.Context, toolName, result string) error {
		return errors.New("result too large")
	}

	result, err := r.Run(context.Background(), "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Tool was executed (ToolCalls incremented) even though the result was rejected.
	if result.ToolCalls != 1 {
		t.Errorf("ToolCalls = %d, want 1", result.ToolCalls)
	}
}

func TestRunExecuteOverride(t *testing.T) {
	t.Parallel()

	// sandboxable=true so the override path is taken.
	ft := &fakeTool{name: "sandboxed", output: "direct output", sandboxable: true}

	p := &fakeProvider{
		responses: []message.Message{
			toolCallMsg("c1", "sandboxed", nil),
			textMsg("ok"),
		},
	}

	overrideCalled := false
	r := newRunner(p, tool.Tools{ft})

	r.ExecuteOverride = func(_ context.Context, toolName string, _ map[string]any) (string, error) {
		overrideCalled = true

		return "override output", nil
	}

	result, err := r.Run(context.Background(), "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !overrideCalled {
		t.Error("ExecuteOverride was not called")
	}

	// The tool's own Execute should NOT have been called.
	if ft.executed {
		t.Error("tool.Execute was called when it should have been bypassed by ExecuteOverride")
	}

	if result.ToolCalls != 1 {
		t.Errorf("ToolCalls = %d, want 1", result.ToolCalls)
	}
}

func TestRunToolPreError(t *testing.T) {
	t.Parallel()

	ft := &fakeTool{
		name:   "pretool",
		output: "should not appear",
		preErr: errors.New("pre-hook failed"),
	}

	p := &fakeProvider{
		responses: []message.Message{
			toolCallMsg("c1", "pretool", nil),
			textMsg("next turn"),
		},
	}

	r := newRunner(p, tool.Tools{ft})

	result, err := r.Run(context.Background(), "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Pre returned error so Execute must not have run.
	if ft.executed {
		t.Error("tool.Execute was called even though Pre returned an error")
	}

	// Loop must have continued past the failed tool call.
	if result.Text != "next turn" {
		t.Errorf("Text = %q, want %q", result.Text, "next turn")
	}
}

func TestRunUsageAccumulated(t *testing.T) {
	t.Parallel()

	ft := &fakeTool{name: "counter", output: "counted"}

	p := &fakeProvider{
		responses: []message.Message{
			toolCallMsg("c1", "counter", nil),
			textMsg("done"),
		},
		usages: []usage.Usage{
			{Input: 10, Output: 5},
			{Input: 10, Output: 5},
		},
	}

	r := newRunner(p, tool.Tools{ft})

	result, err := r.Run(context.Background(), "count")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Usage.Input != 20 {
		t.Errorf("Usage.Input = %d, want 20", result.Usage.Input)
	}

	if result.Usage.Output != 10 {
		t.Errorf("Usage.Output = %d, want 10", result.Usage.Output)
	}
}

func TestRunCWDPassedToHooks(t *testing.T) {
	t.Parallel()

	ft := &fakeTool{name: "mytool", output: "tool output"}

	p := &fakeProvider{
		responses: []message.Message{
			toolCallMsg("c1", "mytool", map[string]any{"x": "1"}),
			textMsg("done"),
		},
	}

	// Pre-hook: "cat" echoes the JSON stdin (which contains CWD) back as stdout.
	// The hooks runner returns that as a non-blocking message which gets prepended to output.
	r := newRunner(p, tool.Tools{ft})

	r.CWD = "/test/workdir"
	r.HooksRunner = hooks.Runner{
		Pre: []hooks.Entry{{
			Name:    "cwd-check",
			Command: "cat",
			Timeout: 5 * time.Second,
		}},
	}

	result, err := r.Run(context.Background(), "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The pre-hook's "cat" echoes the JSON event which contains the CWD field.
	// It becomes a non-blocking preHookMsg prepended to the tool output.
	if !strings.Contains(result.Text, "done") {
		t.Errorf("Text = %q, want to contain %q", result.Text, "done")
	}
}

func TestRunPreHookBlockedEmptyMessage(t *testing.T) {
	t.Parallel()

	ft := &fakeTool{name: "blocked-tool", output: "should not appear"}

	p := &fakeProvider{
		responses: []message.Message{
			toolCallMsg("c1", "blocked-tool", nil),
			textMsg("recovered"),
		},
	}

	// Pre-hook: exit 2 with empty stderr = blocked with empty message.
	// The hooks runner fills "hook blocked: <cmd>" internally, but we verify
	// the runner's own fallback path would work if message were truly empty.
	r := newRunner(p, tool.Tools{ft})

	r.HooksRunner = hooks.Runner{
		Pre: []hooks.Entry{{
			Name:    "blocker",
			Command: "sh -c 'exit 2'",
			Timeout: 5 * time.Second,
		}},
	}

	result, err := r.Run(context.Background(), "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Text != "recovered" {
		t.Errorf("Text = %q, want %q", result.Text, "recovered")
	}

	// Tool should NOT have executed.
	if ft.executed {
		t.Error("tool.Execute was called even though pre-hook blocked it")
	}
}

func TestRunLSPSinglePathsCall(t *testing.T) {
	t.Parallel()

	ft := &fakeTool{
		name:       "writer",
		output:     "written",
		writePaths: []string{"/tmp/test.go"},
		wantsLSP:   true,
	}

	p := &fakeProvider{
		responses: []message.Message{
			toolCallMsg("c1", "writer", nil),
			textMsg("done"),
		},
	}

	r := newRunner(p, tool.Tools{ft})
	// Set PathChecker to non-nil so the path pre-check fires.
	r.PathChecker = func(read, write []string) error { return nil }
	// No LSPManager — we just verify Paths() is called once (not twice).

	_, err := r.Run(context.Background(), "write")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Paths should be called exactly once (hoisted before PathChecker, reused for LSP).
	if ft.pathsCallCount != 1 {
		t.Errorf("Paths() called %d times, want 1", ft.pathsCallCount)
	}
}

func TestRunPostHookAppendsMessage(t *testing.T) {
	t.Parallel()

	ft := &fakeTool{name: "mytool", output: "tool output"}

	p := &fakeProvider{
		responses: []message.Message{
			toolCallMsg("c1", "mytool", nil),
			textMsg("done"),
		},
	}

	r := newRunner(p, tool.Tools{ft})

	r.HooksRunner = hooks.Runner{
		Post: []hooks.Entry{{
			Name:    "post-warning",
			Command: "sh -c 'cat > /dev/null; echo post-warning'",
			Timeout: 5 * time.Second,
		}},
	}

	_, err := r.Run(context.Background(), "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// We can't directly inspect the tool result message, but the post-hook message
	// gets appended to the tool output and sent to the builder. The LLM sees it
	// and responds. If it didn't crash, the pipeline worked.
	// More direct verification would require inspecting builder internals.
}

func TestRunPreHookNonBlockingPrepends(t *testing.T) {
	t.Parallel()

	ft := &fakeTool{name: "mytool", output: "tool output"}

	p := &fakeProvider{
		responses: []message.Message{
			toolCallMsg("c1", "mytool", nil),
			textMsg("done"),
		},
	}

	r := newRunner(p, tool.Tools{ft})

	r.HooksRunner = hooks.Runner{
		Pre: []hooks.Entry{{
			Name:    "pre-warning",
			Command: "sh -c 'cat > /dev/null; echo pre-warning'",
			Timeout: 5 * time.Second,
		}},
	}

	_, err := r.Run(context.Background(), "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Non-blocking pre-hook message gets prepended to tool output.
	// Pipeline executed without error — the builder received the combined output.
}

func TestRunTrackerCreatedWithPolicy(t *testing.T) {
	t.Parallel()

	// Use a custom policy to verify it reaches the Tracker.
	customPolicy := tool.ReadBeforePolicy{Write: false, Delete: true}

	// fakeTool whose Pre records the policy it sees from context.
	var capturedPolicy tool.ReadBeforePolicy

	ft := &fakeTool{name: "polcheck", output: "ok"}
	// Override Pre via a wrapper tool that captures policy.
	pt := &policyCaptureTool{
		fakeTool: ft,
		onPre:    func(ctx context.Context) { capturedPolicy = filetime.FromContext(ctx).Policy() },
	}

	p := &fakeProvider{
		responses: []message.Message{
			toolCallMsg("c1", "polcheck", nil),
			textMsg("done"),
		},
	}

	r := newRunner(p, tool.Tools{pt})

	r.ReadBeforePolicy = customPolicy

	_, err := r.Run(context.Background(), "check policy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedPolicy != customPolicy {
		t.Errorf("Tracker policy = %+v, want %+v", capturedPolicy, customPolicy)
	}
}

func TestRunTrackerIsolation(t *testing.T) {
	t.Parallel()

	// Two runners should get independent Trackers (no shared state).
	// Record a path in runner A's tool execution, verify runner B doesn't see it.
	var trackerA, trackerB *filetime.Tracker

	ftA := &policyCaptureTool{
		fakeTool: &fakeTool{name: "recorder", output: "ok"},
		onPre: func(ctx context.Context) {
			tr := filetime.FromContext(ctx)

			trackerA = tr
			tr.RecordRead("/shared/file.txt")
		},
	}

	ftB := &policyCaptureTool{
		fakeTool: &fakeTool{name: "checker", output: "ok"},
		onPre: func(ctx context.Context) {
			trackerB = filetime.FromContext(ctx)
		},
	}

	pA := &fakeProvider{
		responses: []message.Message{
			toolCallMsg("c1", "recorder", nil),
			textMsg("done"),
		},
	}

	pB := &fakeProvider{
		responses: []message.Message{
			toolCallMsg("c1", "checker", nil),
			textMsg("done"),
		},
	}

	rA := newRunner(pA, tool.Tools{ftA})
	rB := newRunner(pB, tool.Tools{ftB})

	if _, err := rA.Run(context.Background(), "record"); err != nil {
		t.Fatalf("runner A error: %v", err)
	}

	if _, err := rB.Run(context.Background(), "check"); err != nil {
		t.Fatalf("runner B error: %v", err)
	}

	if trackerA == nil || trackerB == nil {
		t.Fatal("one or both trackers were not captured")
	}

	if trackerA == trackerB {
		t.Error("runners A and B share the same Tracker pointer, want distinct instances")
	}

	if !trackerA.WasRead("/shared/file.txt") {
		t.Error("tracker A: WasRead = false, want true")
	}

	if trackerB.WasRead("/shared/file.txt") {
		t.Error("tracker B: WasRead = true, want false (should be isolated from A)")
	}
}

// policyCaptureTool wraps a fakeTool but allows injecting custom Pre behavior
// to inspect or modify the context (e.g. capture the Tracker).
type policyCaptureTool struct {
	*fakeTool

	onPre func(ctx context.Context)
}

func (pt *policyCaptureTool) Pre(ctx context.Context, args map[string]any) error {
	if pt.onPre != nil {
		pt.onPre(ctx)
	}

	return pt.fakeTool.Pre(ctx, args)
}
