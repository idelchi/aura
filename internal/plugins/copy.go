package plugins

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"strings"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// Vendor runs "go mod tidy && go mod vendor" in dir.
// Requires the Go toolchain to be available in PATH.
func Vendor(dir string) error {
	if err := runGoCmd(dir, "go", "mod", "tidy"); err != nil {
		return err
	}

	if err := runGoCmd(dir, "go", "mod", "vendor"); err != nil {
		return err
	}

	return nil
}

// runGoCmd runs a Go command in dir, capturing stderr for error messages and debug logging.
func runGoCmd(dir string, args ...string) error {
	label := strings.Join(args, " ")
	debug.Log("[vendor] running '%s' in %s", label, dir)

	cmd := exec.Command(args[0], args[1:]...)

	cmd.Dir = dir

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}

	cmd.Stdout = nil // tidy/vendor write progress to stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}

	// Capture stderr for both debug logging and error reporting.
	var stderrLines []string

	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()

		stderrLines = append(stderrLines, line)
		debug.Log("[vendor] %s: %s", label, line)
	}

	if err := cmd.Wait(); err != nil {
		if len(stderrLines) > 0 {
			return fmt.Errorf("%s: %w\n%s", label, err, strings.Join(stderrLines, "\n"))
		}

		return fmt.Errorf("%s: %w", label, err)
	}

	debug.Log("[vendor] '%s' done in %s", label, dir)

	return nil
}

// copyPluginToGoPath copies a plugin's .go files and vendor/ directory into dst.
func copyPluginToGoPath(pluginDir, dst string) error {
	dstDir := folder.New(dst)

	if err := dstDir.Create(); err != nil {
		return err
	}

	srcDir := folder.New(pluginDir)

	entries, err := srcDir.ListFiles()
	if err != nil {
		return err
	}

	var found bool

	for _, e := range entries {
		if e.Extension() != "go" {
			continue
		}

		found = true

		data, err := e.Read()
		if err != nil {
			return err
		}

		if err := dstDir.WithFile(e.Base()).Write(data, 0o644); err != nil {
			return err
		}
	}

	if !found {
		return fmt.Errorf("no .go files found in %s", pluginDir)
	}

	vendorDir := folder.New(pluginDir, "vendor")
	if vendorDir.Exists() {
		if err := copyDirRecursive(vendorDir.Path(), folder.New(dst, "vendor").Path()); err != nil {
			return fmt.Errorf("copying vendor: %w", err)
		}
	}

	return nil
}

// readModulePath reads the module path from go.mod in the given directory.
func readModulePath(dir string) (string, error) {
	data, err := folder.New(dir).WithFile("go.mod").Read()
	if err != nil {
		return "", fmt.Errorf("reading go.mod: %w (every plugin must have a go.mod)", err)
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module")), nil
		}
	}

	return "", errors.New("no module directive found in go.mod")
}

// copyDirRecursive copies all .go files and subdirectories from src to dst.
func copyDirRecursive(src, dst string) error {
	return folder.New(src).Walk(func(p file.File, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, _ := p.RelativeTo(src)
		target := folder.New(dst, rel.Path())

		if d.IsDir() {
			return target.Create()
		}

		if file.New(d.Name()).Extension() != "go" {
			return nil
		}

		if strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		data, err := p.Read()
		if err != nil {
			return err
		}

		return target.AsFile().Write(data, 0o644)
	})
}
