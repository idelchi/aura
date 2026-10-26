package mcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/idelchi/aura/pkg/sandbox"
)

// testServerEnv makes the test binary act as an MCP stdio server.
const testServerEnv = "AURA_MCP_TEST_SERVER"

// TestMain lets the test binary serve as the sandbox launcher and as a stdio MCP server.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == sandbox.ExecArg {
		sandbox.ExecMain(os.Args[2:])
	}

	if os.Getenv(testServerEnv) == "1" {
		serveTestServer()

		return
	}

	os.Exit(m.Run())
}

// serveTestServer exposes tools that write files, fail, flood stderr, spawn a child and sleep.
func serveTestServer() {
	s := server.NewMCPServer("aura-test", "0.0.0")

	text := func(format string, args ...any) *mcplib.CallToolResult {
		return mcplib.NewToolResultText(fmt.Sprintf(format, args...))
	}

	s.AddTool(mcplib.NewTool("write", mcplib.WithString("path")), func(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		if err := os.WriteFile(req.GetString("path", ""), []byte("written"), 0o600); err != nil {
			return mcplib.NewToolResultError(err.Error()), nil
		}

		return text("ok"), nil
	})
	s.AddTool(mcplib.NewTool("fail"), func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return mcplib.NewToolResultError("semantic failure: revision changed"), nil
	})
	s.AddTool(mcplib.NewTool("flood"), func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		line := strings.Repeat("x", 1023) + "\n"
		for range 2048 { // 2 MiB, far beyond a pipe buffer
			os.Stderr.WriteString(line)
		}

		return text("flooded"), nil
	})
	s.AddTool(mcplib.NewTool("spawn"), func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		child := exec.Command("sleep", "300")
		if err := child.Start(); err != nil {
			return mcplib.NewToolResultError(err.Error()), nil
		}

		return text("%d", child.Process.Pid), nil
	})
	s.AddTool(mcplib.NewTool("sleep"), func(ctx context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		select {
		case <-ctx.Done():
			return mcplib.NewToolResultError("cancelled"), nil
		case <-time.After(5 * time.Minute):
			return text("slept"), nil
		}
	})

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// testServer returns a stdio configuration that runs this test binary as the MCP server.
func testServer(t *testing.T) Server {
	t.Helper()

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv(testServerEnv, "1")

	return Server{Command: exe, Required: true, Timeout: 20 * time.Second}
}

// call invokes a test tool and returns its output or error.
func call(t *testing.T, ctx context.Context, sess *Session, name string, args map[string]any) (string, error) {
	t.Helper()

	for _, tl := range sess.Tools() {
		if tl.mcpTool.Name == name {
			return tl.Execute(ctx, args)
		}
	}

	t.Fatalf("tool %s not offered", name)

	return "", nil
}

func connect(t *testing.T, srv Server, confinement Confinement) *Session {
	t.Helper()

	sess, err := NewSession(t.Context(), "test", srv, confinement)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	t.Cleanup(func() { sess.Close() })

	return sess
}

func TestToolErrorResultIsError(t *testing.T) {
	sess := connect(t, testServer(t), Confinement{})

	out, err := call(t, t.Context(), sess, "fail", nil)
	if err == nil || !strings.Contains(err.Error(), "revision changed") || out != "" {
		t.Fatalf("fail tool = %q, %v; want the isError text as an error", out, err)
	}
}

func TestStderrFloodDoesNotBlock(t *testing.T) {
	sess := connect(t, testServer(t), Confinement{})

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	if out, err := call(t, ctx, sess, "flood", nil); err != nil || out != "flooded" {
		t.Fatalf("flood = %q, %v", out, err)
	}
}

func TestCancelledCallReturns(t *testing.T) {
	sess := connect(t, testServer(t), Confinement{})

	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()

	start := time.Now()

	if _, err := call(t, ctx, sess, "sleep", nil); err == nil {
		t.Fatal("cancelled call succeeded")
	}

	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("cancelled call returned after %v", elapsed)
	}
}

func TestUnsetVariablesFailExpansion(t *testing.T) {
	srv := testServer(t)
	srv.Args = []string{"${AURA_MCP_TEST_UNSET_VARIABLE}"}

	if _, err := NewSession(t.Context(), "test", srv, Confinement{}); err == nil ||
		!strings.Contains(err.Error(), "AURA_MCP_TEST_UNSET_VARIABLE") {
		t.Fatalf("NewSession = %v, want unset variable error", err)
	}

	t.Setenv("AURA_MCP_TEST_EMPTY", "")

	srv.Args = []string{"${AURA_MCP_TEST_EMPTY}"}

	expanded, err := srv.Expanded()
	if err != nil || expanded.Args[0] != "" {
		t.Fatalf("set-but-empty variable = %v, %v", expanded.Args, err)
	}
}

func TestRequiredFailureIsReported(t *testing.T) {
	results := ConnectAll(t.Context(), map[string]Server{
		"missing":  {Command: filepath.Join(t.TempDir(), "absent"), Required: true, Timeout: 5 * time.Second},
		"optional": {Command: filepath.Join(t.TempDir(), "absent"), Timeout: 5 * time.Second},
	}, Confinement{}, nil)

	err := RequiredFailures(results)
	if err == nil || !strings.Contains(err.Error(), "missing") || strings.Contains(err.Error(), "optional") {
		t.Fatalf("RequiredFailures = %v", err)
	}
}

func TestSandboxRequiresStdio(t *testing.T) {
	if err := (Server{Type: "http", URL: "http://localhost", Sandbox: true}).Validate(); err == nil {
		t.Fatal("http server with sandbox validated")
	}

	if err := (Server{Command: "x", Sandbox: true}).Validate(); err != nil {
		t.Fatalf("stdio sandbox: %v", err)
	}
}

// confinedServer launches the test server confined to a writable workspace.
func confinedServer(t *testing.T) (*Session, string) {
	t.Helper()

	if !sandbox.IsAvailable() {
		t.Skip("Landlock is not available")
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	workspace := t.TempDir()

	srv := testServer(t)
	srv.Sandbox = true

	confinement := Confinement{
		WorkDir:   workspace,
		ReadOnly:  []string{"/usr", "/bin", "/lib", "/lib64", "/etc", filepath.Dir(exe)},
		ReadWrite: []string{".", "/dev/null"},
	}

	return connect(t, srv, confinement), workspace
}

func TestSandboxedServerConfinesWrites(t *testing.T) {
	sess, workspace := confinedServer(t)

	if out, err := call(t, t.Context(), sess, "write", map[string]any{"path": "inside.txt"}); err != nil || out != "ok" {
		t.Fatalf("write inside workspace = %q, %v", out, err)
	}

	if _, err := os.Stat(filepath.Join(workspace, "inside.txt")); err != nil {
		t.Fatalf("relative write did not land in the workspace: %v", err)
	}

	outside := t.TempDir()

	if _, err := call(t, t.Context(), sess, "write", map[string]any{"path": filepath.Join(outside, "escape.txt")}); err == nil ||
		!strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("write outside workspace = %v, want permission denied", err)
	}

	if err := os.Symlink(outside, filepath.Join(workspace, "link")); err != nil {
		t.Fatal(err)
	}

	if _, err := call(t, t.Context(), sess, "write", map[string]any{"path": "link/through.txt"}); err == nil {
		t.Fatal("write through a symlink escaped the workspace")
	}

	for _, name := range []string{"escape.txt", "through.txt"} {
		if _, err := os.Stat(filepath.Join(outside, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s exists outside the workspace", name)
		}
	}
}

func TestSandboxedCloseKillsServerGroup(t *testing.T) {
	sess, _ := confinedServer(t)

	out, err := call(t, t.Context(), sess, "spawn", nil)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	pid, err := strconv.Atoi(out)
	if err != nil {
		t.Fatal(err)
	}

	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("spawned child not running: %v", err)
	}

	sess.Close()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		// Reap if it became our zombie; otherwise check existence.
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}

		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("server child %d survived session close", pid)
}

func TestSandboxedServerWithoutRulesDoesNotStart(t *testing.T) {
	if !sandbox.IsAvailable() {
		t.Skip("Landlock is not available")
	}

	srv := testServer(t)
	srv.Sandbox = true

	// No read access anywhere: the server binary cannot even be executed, so the
	// connection fails instead of running unconfined.
	if _, err := NewSession(t.Context(), "test", srv, Confinement{WorkDir: t.TempDir()}); err == nil {
		t.Fatal("server started without any permitted paths")
	}
}
