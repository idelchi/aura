package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/idelchi/aura/internal/config"

	"go.yaml.in/yaml/v4"
)

// approvalsFile is the canonical path relative to the config dir.
const approvalsRelPath = "config/rules/approvals.yaml"

// readApprovals reads and unmarshals the approvals file from configDir.
func readApprovals(t *testing.T, configDir string) struct {
	ToolAuto []string `yaml:"tool_auto"`
} {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(configDir, approvalsRelPath))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var out struct {
		ToolAuto []string `yaml:"tool_auto"`
	}

	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	return out
}

func TestSaveToolApproval(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	if err := config.SaveToolApproval(dir, "Bash:git*"); err != nil {
		t.Fatalf("SaveToolApproval: %v", err)
	}

	got := readApprovals(t, dir)

	if len(got.ToolAuto) != 1 {
		t.Fatalf("ToolAuto len = %d, want 1; got %v", len(got.ToolAuto), got.ToolAuto)
	}

	if got.ToolAuto[0] != "Bash:git*" {
		t.Errorf("ToolAuto[0] = %q, want %q", got.ToolAuto[0], "Bash:git*")
	}
}

func TestSaveToolApprovalDedup(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Save the same pattern twice — file should only contain one entry.
	if err := config.SaveToolApproval(dir, "Bash:git*"); err != nil {
		t.Fatalf("first SaveToolApproval: %v", err)
	}

	if err := config.SaveToolApproval(dir, "Bash:git*"); err != nil {
		t.Fatalf("second SaveToolApproval: %v", err)
	}

	got := readApprovals(t, dir)

	if len(got.ToolAuto) != 1 {
		t.Errorf("ToolAuto len = %d, want 1 (dedup); got %v", len(got.ToolAuto), got.ToolAuto)
	}
}

func TestSaveToolApprovalAppend(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	if err := config.SaveToolApproval(dir, "Bash:git*"); err != nil {
		t.Fatalf("first SaveToolApproval: %v", err)
	}

	if err := config.SaveToolApproval(dir, "Write"); err != nil {
		t.Fatalf("second SaveToolApproval: %v", err)
	}

	got := readApprovals(t, dir)

	if len(got.ToolAuto) != 2 {
		t.Fatalf("ToolAuto len = %d, want 2; got %v", len(got.ToolAuto), got.ToolAuto)
	}

	// Result must be sorted: "Bash:git*" < "Write" lexicographically.
	if got.ToolAuto[0] != "Bash:git*" {
		t.Errorf("ToolAuto[0] = %q, want %q", got.ToolAuto[0], "Bash:git*")
	}

	if got.ToolAuto[1] != "Write" {
		t.Errorf("ToolAuto[1] = %q, want %q", got.ToolAuto[1], "Write")
	}
}

func TestSaveToolApprovalCreatesDirectories(t *testing.T) {
	t.Parallel()

	// Use a deeply nested temp dir that doesn't yet exist inside TempDir.
	base := t.TempDir()
	dir := filepath.Join(base, "project", "workspace")

	if err := config.SaveToolApproval(dir, "Read"); err != nil {
		t.Fatalf("SaveToolApproval: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, approvalsRelPath)); err != nil {
		t.Errorf("approvals file not created: %v", err)
	}
}

func TestSaveToolApprovalMultiplePatterns(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	patterns := []string{"Write", "mcp__*", "Bash:git*", "Read"}
	for _, p := range patterns {
		if err := config.SaveToolApproval(dir, p); err != nil {
			t.Fatalf("SaveToolApproval(%q): %v", p, err)
		}
	}

	got := readApprovals(t, dir)

	if len(got.ToolAuto) != len(patterns) {
		t.Fatalf("ToolAuto len = %d, want %d; got %v", len(got.ToolAuto), len(patterns), got.ToolAuto)
	}

	// All saved patterns must be sorted in the file.
	want := []string{"Bash:git*", "Read", "Write", "mcp__*"}
	for i, v := range want {
		if got.ToolAuto[i] != v {
			t.Errorf("ToolAuto[%d] = %q, want %q", i, got.ToolAuto[i], v)
		}
	}
}

func TestSaveToolApprovalConcurrent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	const n = 20

	var wg sync.WaitGroup

	wg.Add(n)

	for i := range n {
		go func(idx int) {
			defer wg.Done()

			pattern := fmt.Sprintf("Tool%d", idx)
			if err := config.SaveToolApproval(dir, pattern); err != nil {
				t.Errorf("SaveToolApproval(%q): %v", pattern, err)
			}
		}(i)
	}

	wg.Wait()

	got := readApprovals(t, dir)

	if len(got.ToolAuto) != n {
		t.Errorf(
			"concurrent SaveToolApproval: got %d patterns, want %d; patterns: %v",
			len(got.ToolAuto),
			n,
			got.ToolAuto,
		)
	}
}

func TestSaveToolApprovalPathQualified(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	patterns := []string{"Write:/tmp/*", "Read:/etc/*", "Write:/home/*"}
	for _, p := range patterns {
		if err := config.SaveToolApproval(dir, p); err != nil {
			t.Fatalf("SaveToolApproval(%q): %v", p, err)
		}
	}

	got := readApprovals(t, dir)

	if len(got.ToolAuto) != 3 {
		t.Fatalf("ToolAuto len = %d, want 3; got %v", len(got.ToolAuto), got.ToolAuto)
	}

	// Sorted: Read:/etc/*, Write:/home/*, Write:/tmp/*
	want := []string{"Read:/etc/*", "Write:/home/*", "Write:/tmp/*"}
	for i, v := range want {
		if got.ToolAuto[i] != v {
			t.Errorf("ToolAuto[%d] = %q, want %q", i, got.ToolAuto[i], v)
		}
	}
}
