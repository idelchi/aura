package config

import (
	"fmt"
	"slices"
	"strings"

	"github.com/gofrs/flock"

	"github.com/idelchi/aura/pkg/yamlutil"
	"github.com/idelchi/godyl/pkg/path/files"
	"github.com/idelchi/godyl/pkg/path/folder"

	"go.yaml.in/yaml/v4"
)

// Approvals holds approval patterns loaded from a single rules file.
type Approvals struct {
	ToolAuto []string `yaml:"tool_auto"`
}

// ApprovalRules holds merged approval patterns with provenance (project vs global).
type ApprovalRules struct {
	Project Approvals
	Global  Approvals
}

// Load populates approval rules from the discovered rules files.
// Files under globalHome are tagged Global, others Project.
func (r *ApprovalRules) Load(ff files.Files, globalHome string) error {
	for _, f := range ff {
		data, err := f.Read()
		if err != nil {
			return fmt.Errorf("reading %s: %w", f, err)
		}

		var a Approvals
		if err := yamlutil.StrictUnmarshal(data, &a); err != nil {
			return fmt.Errorf("parsing %s: %w", f, err)
		}

		if globalHome != "" && strings.HasPrefix(f.Path(), globalHome) {
			r.Global.ToolAuto = append(r.Global.ToolAuto, a.ToolAuto...)
		} else {
			r.Project.ToolAuto = append(r.Project.ToolAuto, a.ToolAuto...)
		}
	}

	slices.Sort(r.Project.ToolAuto)

	r.Project.ToolAuto = slices.Compact(r.Project.ToolAuto)

	slices.Sort(r.Global.ToolAuto)

	r.Global.ToolAuto = slices.Compact(r.Global.ToolAuto)

	return nil
}

// SaveToolApproval appends a tool auto-approval pattern to the rules file at configDir/config/rules/approvals.yaml.
// Creates the directory and file if they don't exist. Deduplicates entries.
// Uses advisory file locking for concurrent-instance safety.
func SaveToolApproval(configDir, pattern string) error {
	dir := folder.New(configDir, "config", "rules")
	path := dir.WithFile("approvals.yaml")

	// Create dir first — lock file needs the directory to exist.
	if err := dir.Create(); err != nil {
		return err
	}

	// Advisory lock for concurrent-instance safety.
	fileLock := flock.New(path.Path() + ".lock")
	if err := fileLock.Lock(); err != nil {
		return fmt.Errorf("acquiring lock: %w", err)
	}

	defer fileLock.Unlock()

	// Load existing file if present.
	var a Approvals

	if data, err := path.Read(); err == nil {
		if err := yamlutil.StrictUnmarshal(data, &a); err != nil {
			return fmt.Errorf("parsing existing approvals %s: %w", path, err)
		}
	}

	// Append and deduplicate.
	a.ToolAuto = append(a.ToolAuto, pattern)
	slices.Sort(a.ToolAuto)

	a.ToolAuto = slices.Compact(a.ToolAuto)

	// Marshal and write.
	data, err := yaml.Marshal(&a)
	if err != nil {
		return err
	}

	return path.Write(data, 0o644)
}
