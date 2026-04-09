package sandbox

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/landlock-lsm/go-landlock/landlock"
	"github.com/landlock-lsm/go-landlock/landlock/syscall"
	"github.com/samber/lo"

	"github.com/idelchi/godyl/pkg/path/file"
)

// Sandbox defines the set of filesystem paths and their access levels for Landlock enforcement.
type Sandbox struct {
	ReadOnly  types
	ReadWrite types

	// WorkDir is the effective working directory, used in String() for display.
	WorkDir string

	// intent tracks all configured paths including non-existent ones.
	// Landlock requires existing paths, but CanRead/CanWrite are pure prefix
	// checks that should reflect the full declared intent.
	intent struct {
		readOnly  types
		readWrite types
	}
}

// types holds the resolved file and folder paths for a given access level.
type types struct {
	Folders []string
	Files   []string
}

// String returns the formatted path listing for this access level.
func (t *types) String() string {
	const (
		folderIcon = "📁"
		fileIcon   = "📄"
	)

	var b strings.Builder

	for _, dir := range t.Folders {
		fmt.Fprintf(&b, "    - %-40s %s\n", dir, folderIcon)
	}

	for _, file := range t.Files {
		fmt.Fprintf(&b, "    - %-40s %s\n", file, fileIcon)
	}

	return b.String()
}

// Length returns the total number of registered files and folders.
func (t *types) Length() int {
	return len(t.Folders) + len(t.Files)
}

// IsAvailable reports whether the kernel supports Landlock.
func IsAvailable() bool {
	_, err := syscall.LandlockGetABIVersion()

	return err == nil
}

// NumberOfRules returns the total number of registered read and write rules.
func (s *Sandbox) NumberOfRules() int {
	return s.ReadOnly.Length() + s.ReadWrite.Length()
}

// AddReadOnly registers paths for read-only access within the sandbox.
func (s *Sandbox) AddReadOnly(paths ...string) {
	s.addPaths(&s.ReadOnly, &s.intent.readOnly, paths)
}

// AddReadWrite registers paths for read-write access within the sandbox.
func (s *Sandbox) AddReadWrite(paths ...string) {
	s.addPaths(&s.ReadWrite, &s.intent.readWrite, paths)
}

// HasRules reports whether any read or write rules have been registered.
func (s *Sandbox) HasRules() bool {
	return s.NumberOfRules() > 0
}

// Apply enforces the sandbox by applying all registered Landlock rules.
func (s *Sandbox) Apply() error {
	rules := make([]landlock.Rule, 0, s.NumberOfRules())

	for _, dir := range s.ReadOnly.Folders {
		rules = append(rules, landlock.RODirs(dir))
	}

	for _, file := range s.ReadOnly.Files {
		rules = append(rules, landlock.ROFiles(file))
	}

	for _, dir := range s.ReadWrite.Folders {
		rules = append(rules, landlock.RWDirs(dir))
	}

	for _, file := range s.ReadWrite.Files {
		rules = append(rules, landlock.RWFiles(file))
	}

	if err := landlock.V5.BestEffort().RestrictPaths(rules...); err != nil {
		return fmt.Errorf("applying Landlock restrictions: %w", err)
	}

	return nil
}

// CanRead reports whether the given path falls within any configured read-only or read-write path.
// Checks against the full declared intent, including non-existent paths.
func (s *Sandbox) CanRead(path string) bool {
	return s.matchesAny(path, &s.intent.readOnly) || s.matchesAny(path, &s.intent.readWrite)
}

// CanWrite reports whether the given path falls within any configured read-write path.
// Checks against the full declared intent, including non-existent paths.
func (s *Sandbox) CanWrite(path string) bool {
	return s.matchesAny(path, &s.intent.readWrite)
}

// String returns a human-readable summary of the sandbox restrictions.
func (s *Sandbox) String(omitReadOnly ...bool) string {
	var sections string

	if s.ReadOnly.Length() > 0 && (len(omitReadOnly) == 0 || !omitReadOnly[0]) {
		sections += " Read Only:\n" + s.ReadOnly.String()
	}

	if s.ReadWrite.Length() > 0 {
		sections += " Read Write:\n" + s.ReadWrite.String()
	}

	if !s.HasRules() {
		sections = " - No read or write rights to anything\n"
	}

	cwd := s.WorkDir

	sections = strings.TrimSpace(sections)

	return heredoc.Docf(`
		Landlock Sandbox Restrictions:
		%s

		If you encountered 'permission denied' errors, you are most probably hitting the intended sandbox restrictions.

		You are currently in: %q
	`, sections, cwd)
}

// addPaths resolves each path and adds it to both the Landlock rules (dst) and
// the intent list (intentDst). Non-existent paths are added to intent only
// (as folders, since we can't stat them) — Landlock requires existing paths.
func (s *Sandbox) addPaths(dst, intentDst *types, paths []string) {
	for _, path := range paths {
		f := file.New(path).Expanded()
		if !f.IsAbs() {
			f = file.New(s.WorkDir, f.Path())
		}

		abs := f.Path()

		info, err := f.Info()
		if err != nil {
			// Path doesn't exist — record intent as folder for prefix matching.
			intentDst.Folders = append(intentDst.Folders, abs)

			continue
		}

		if info.IsDir() {
			dst.Folders = append(dst.Folders, abs)
			intentDst.Folders = append(intentDst.Folders, abs)
		} else {
			dst.Files = append(dst.Files, abs)
			intentDst.Files = append(intentDst.Files, abs)
		}
	}

	dst.Folders = lo.Uniq(dst.Folders)
	dst.Files = lo.Uniq(dst.Files)
	intentDst.Folders = lo.Uniq(intentDst.Folders)
	intentDst.Files = lo.Uniq(intentDst.Files)
}

// matchesAny checks whether path is covered by any folder or file entry in t.
func (s *Sandbox) matchesAny(path string, t *types) bool {
	f := file.New(path).Expanded()
	if !f.IsAbs() {
		f = file.New(s.WorkDir, f.Path())
	}

	abs := f.Path()

	for _, dir := range t.Folders {
		if abs == dir || strings.HasPrefix(abs, dir+string(filepath.Separator)) {
			return true
		}
	}

	return slices.Contains(t.Files, abs)
}
