package message_test

import (
	"strings"
	"testing"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/tool/call"
)

// fakeEstimate returns string length as a rough token count, good enough for tests.
func fakeEstimate(s string) int { return len(s) }

// makeCall builds a call.Call with the given arguments.
func makeCall(name string, args map[string]any) call.Call {
	c := call.Call{Name: name, ID: "call-" + name}
	c.SetArgs(args)

	return c
}

// assistantWithCalls creates an assistant message carrying tool calls.
func assistantWithCalls(tokens int, calls ...call.Call) message.Message {
	return message.Message{
		Role:   roles.Assistant,
		Calls:  calls,
		Tokens: message.Tokens{Total: tokens},
	}
}

// toolResult creates a tool-role message.
func toolResult(tokens int, content string) message.Message {
	return message.Message{
		Role:    roles.Tool,
		Content: content,
		Tokens:  message.Tokens{Total: tokens},
	}
}

func TestPruneToolResults_PreservesParameterNames(t *testing.T) {
	t.Parallel()

	longCommand := strings.Repeat("x", 500)
	msgs := message.Messages{
		assistantWithCalls(600, makeCall("Bash", map[string]any{
			"command":    longCommand,
			"timeout_ms": 15000,
		})),
		toolResult(100, "ok"),
	}

	// protectTokens=0 means everything is eligible, argThreshold=50 triggers pruning.
	result := msgs.PruneToolResults(0, 50, fakeEstimate)

	pruned := result[0]
	if len(pruned.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(pruned.Calls))
	}

	args := pruned.Calls[0].Arguments

	// Must NOT contain _pruned key.
	if _, ok := args["_pruned"]; ok {
		t.Fatal("pruned args contain '_pruned' key — this is the bug we're fixing")
	}

	// Must preserve original parameter names.
	if _, ok := args["command"]; !ok {
		t.Fatal("pruned args missing 'command' key")
	}

	if _, ok := args["timeout_ms"]; !ok {
		t.Fatal("pruned args missing 'timeout_ms' key")
	}

	// The large value must be truncated (string with sentinel prefix).
	cmdVal, ok := args["command"].(string)
	if !ok {
		t.Fatalf("expected command to be string after truncation, got %T", args["command"])
	}

	if !strings.HasPrefix(cmdVal, "[truncated") {
		t.Errorf("expected command value to start with '[truncated', got %q", cmdVal[:50])
	}

	// The small value must be preserved exactly.
	if args["timeout_ms"] != 15000 {
		t.Errorf("expected timeout_ms=15000, got %v", args["timeout_ms"])
	}
}

func TestPruneToolResults_SmallArgsUntouched(t *testing.T) {
	t.Parallel()

	msgs := message.Messages{
		assistantWithCalls(100, makeCall("Glob", map[string]any{
			"pattern": "*.go",
			"path":    "/home/user",
		})),
		toolResult(50, "found 10 files"),
	}

	// protectTokens=0, argThreshold=500 — args are small enough to survive.
	result := msgs.PruneToolResults(0, 500, fakeEstimate)

	args := result[0].Calls[0].Arguments
	if args["pattern"] != "*.go" {
		t.Errorf("expected pattern='*.go', got %v", args["pattern"])
	}

	if args["path"] != "/home/user" {
		t.Errorf("expected path='/home/user', got %v", args["path"])
	}
}

func TestPruneToolResults_Idempotent(t *testing.T) {
	t.Parallel()

	longContent := strings.Repeat("y", 500)
	msgs := message.Messages{
		assistantWithCalls(600, makeCall("Write", map[string]any{
			"path":    "/tmp/test.go",
			"content": longContent,
		})),
		toolResult(100, "written"),
	}

	// First prune.
	result := msgs.PruneToolResults(0, 50, fakeEstimate)

	// Second prune — must not change anything.
	result2 := result.PruneToolResults(0, 50, fakeEstimate)

	args1 := result[0].Calls[0].Arguments
	args2 := result2[0].Calls[0].Arguments

	for k, v1 := range args1 {
		v2, ok := args2[k]
		if !ok {
			t.Fatalf("key %q lost after second prune", k)
		}

		s1, _ := v1.(string)
		s2, _ := v2.(string)

		if s1 != s2 {
			t.Errorf("key %q changed between prunes:\n  first:  %q\n  second: %q", k, s1, s2)
		}
	}
}

func TestPruneToolResults_ProtectWindowRespected(t *testing.T) {
	t.Parallel()

	longCommand := strings.Repeat("z", 500)
	msgs := message.Messages{
		assistantWithCalls(600, makeCall("Bash", map[string]any{
			"command": longCommand,
		})),
		toolResult(100, "ok"),
	}

	// protectTokens=1000 — both messages fit within the protect window.
	result := msgs.PruneToolResults(1000, 50, fakeEstimate)

	// Args should be untouched — within protect window.
	args := result[0].Calls[0].Arguments
	if args["command"] != longCommand {
		t.Error("args were pruned despite being within the protect window")
	}
}

func TestPruneToolResults_MultipleCallsInOneMessage(t *testing.T) {
	t.Parallel()

	longContent := strings.Repeat("a", 500)
	shortPattern := "*.go"

	msgs := message.Messages{
		assistantWithCalls(800,
			makeCall("Write", map[string]any{
				"path":    "/tmp/f.go",
				"content": longContent,
			}),
			makeCall("Glob", map[string]any{
				"pattern": shortPattern,
			}),
		),
		toolResult(50, "written"),
		toolResult(50, "found files"),
	}

	result := msgs.PruneToolResults(0, 50, fakeEstimate)

	// Write call: content should be truncated, path preserved.
	writeArgs := result[0].Calls[0].Arguments
	if _, ok := writeArgs["_pruned"]; ok {
		t.Fatal("Write call has '_pruned' key")
	}

	if _, ok := writeArgs["path"]; !ok {
		t.Fatal("Write call missing 'path' key")
	}

	if _, ok := writeArgs["content"]; !ok {
		t.Fatal("Write call missing 'content' key")
	}

	// Glob call: small args, should be untouched even though it's in the same message.
	// (The threshold check is per-call based on total call arg size.)
	globArgs := result[0].Calls[1].Arguments
	if globArgs["pattern"] != shortPattern {
		t.Errorf("Glob pattern changed: got %v", globArgs["pattern"])
	}
}
