package hooks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/pkg/yamlutil"
)

func TestGoHooksHandleNoActionsMissingFilesAndSpaces(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AURA_HOOK_TEST_DIR", dir)
	stub := heredoc.Doc(`
		#!/bin/sh
		printf '%s\n' "${*}" >> "${AURA_HOOK_TEST_DIR}/calls"
		case "${*}" in
		  *broken.go*) echo 'real gopls failure' >&2; exit 2 ;;
		  *-exec*) touch "${AURA_HOOK_TEST_DIR}/fixed" ;;
		  *source.fixAll*) [ -f "${AURA_HOOK_TEST_DIR}/fixed" ] || echo 'edit available' ;;
		esac
		exit 0
	`)
	for name, content := range map[string]string{"gopls": stub, "golangci-lint": "#!/bin/sh\nprintf '%s\\n' \"${*}\" >> \"${AURA_HOOK_TEST_DIR}/format-calls\"\n"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(content), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	sample, err := os.ReadFile("../../samples/config/hooks/go.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var definitions config.Hooks
	if err := yamlutil.StrictUnmarshal(sample, &definitions); err != nil {
		t.Fatal(err)
	}
	format := definitions["go:format"]
	fix := definitions["go:fix"]
	// Resolve the fields inherited by this sample without bringing in optional lint.
	fix.Event, fix.Matcher, fix.Files, fix.Silent = format.Event, format.Matcher, format.Files, format.Silent
	runner, err := New(config.Hooks{"go:format": format, "go:fix": fix})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "file with spaces.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"file_path": path}
	result := runner.RunPost(context.Background(), "Edit", input, "Success", dir)
	if result.Blocked || result.Message != "" {
		t.Fatalf("first edit: %+v", result)
	}
	calls, err := os.ReadFile(filepath.Join(dir, "calls"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(calls), "-exec") != 1 || !strings.Contains(string(calls), path) {
		t.Fatalf("calls: %s", calls)
	}
	formatted, err := os.ReadFile(filepath.Join(dir, "format-calls"))
	if err != nil || !strings.Contains(string(formatted), path) {
		t.Fatalf("formatter: %s, %v", formatted, err)
	}
	result = runner.RunPost(context.Background(), "Edit", input, "Success", dir)
	if result.Message != "" {
		t.Fatalf("no actions: %+v", result)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(dir, "calls"))
	if err != nil {
		t.Fatal(err)
	}
	result = runner.RunPost(context.Background(), "Patch", map[string]any{"patch": "*** Begin Patch\n*** Delete File: " + path + "\n*** End Patch"}, "Success", dir)
	if result.Message != "" {
		t.Fatalf("deleted file: %+v", result)
	}
	after, err := os.ReadFile(filepath.Join(dir, "calls"))
	if err != nil || string(before) != string(after) {
		t.Fatal("deleted file reached gopls")
	}
	broken := filepath.Join(dir, "broken.go")
	if err := os.WriteFile(broken, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result = runner.RunPost(context.Background(), "Write", map[string]any{"path": broken}, "Success", dir)
	if result.Blocked || !strings.Contains(result.Message, "real gopls failure") || strings.Contains(result.Message, "hook blocked") {
		t.Fatalf("hidden error: %+v", result)
	}
}
