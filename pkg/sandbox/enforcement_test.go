package sandbox_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/landlock-lsm/go-landlock/landlock/syscall"

	"github.com/idelchi/aura/pkg/sandbox"
)

// Landlock applies to the entire process, so enforcement tests need a child.
func TestApplyWorkspaceBoundary(t *testing.T) {
	const childRoot = "AURA_SANDBOX_TEST_ROOT"

	root := os.Getenv(childRoot)
	if root == "" {
		abi, err := syscall.LandlockGetABIVersion()
		if err != nil || abi < 2 {
			t.Skipf("Landlock ABI 2 required: ABI %d, error %v", abi, err)
		}

		root = t.TempDir()
		outside := mkFile(t, root, "outside/sentinel")
		if err := os.WriteFile(outside, []byte("unchanged"), 0o600); err != nil {
			t.Fatal(err)
		}

		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}

		cmd := exec.Command(executable, "-test.run=^TestApplyWorkspaceBoundary$", "-test.v")
		cmd.Env = append(os.Environ(), childRoot+"="+root)

		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("sandbox child: %v\n%s", err, output)
		}

		data, err := os.ReadFile(outside)
		if err != nil || string(data) != "unchanged" {
			t.Fatalf("outside sentinel changed: %q, %v", data, err)
		}

		return
	}

	workspace := mkDir(t, root, "workspace")
	source := mkFile(t, workspace, "source/file")
	linkSource := mkFile(t, workspace, "source/link-source")
	destination := mkDir(t, workspace, "destination")
	directory := mkDir(t, workspace, "directory")
	outside := filepath.Join(root, "outside/sentinel")
	link := filepath.Join(workspace, "outside-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}

	sb := &sandbox.Sandbox{WorkDir: workspace}
	sb.AddReadOnly("/")
	sb.AddReadWrite(workspace)
	if err := sb.Apply(); err != nil {
		t.Fatal(err)
	}

	moved := filepath.Join(destination, "file")
	for _, operation := range []struct {
		name string
		run  func() error
	}{
		{"create file", func() error { return os.WriteFile(filepath.Join(workspace, "new"), nil, 0o600) }},
		{"rename file across directories", func() error { return os.Rename(source, moved) }},
		{"rename directory across directories", func() error { return os.Rename(directory, filepath.Join(destination, "directory")) }},
		{"hardlink across directories", func() error { return os.Link(linkSource, filepath.Join(workspace, "hardlink")) }},
		{"delete file", func() error { return os.Remove(filepath.Join(workspace, "new")) }},
	} {
		t.Run(operation.name, func(t *testing.T) {
			if err := operation.run(); err != nil {
				t.Fatalf("workspace operation denied: %v", err)
			}
		})
	}

	// Use an independent source so these checks still run if a permitted move fails.
	inside := mkFile(t, workspace, "boundary-source")
	for _, operation := range []struct {
		name string
		run  func() error
	}{
		{"write outside", func() error { return os.WriteFile(outside, []byte("changed"), 0o600) }},
		{"create outside", func() error { return os.WriteFile(filepath.Join(root, "outside/new"), nil, 0o600) }},
		{"delete outside", func() error { return os.Remove(outside) }},
		{"write outward symlink", func() error { return os.WriteFile(link, []byte("changed"), 0o600) }},
		{"move out", func() error { return os.Rename(inside, filepath.Join(root, "outside/moved")) }},
		{"move in", func() error { return os.Rename(outside, filepath.Join(workspace, "imported")) }},
		{"hardlink out", func() error { return os.Link(inside, filepath.Join(root, "outside/hardlink")) }},
		{"hardlink in", func() error { return os.Link(outside, filepath.Join(workspace, "imported-link")) }},
	} {
		t.Run(operation.name, func(t *testing.T) {
			if err := operation.run(); err == nil {
				t.Fatal("operation crossed the workspace boundary")
			}
		})
	}
}
