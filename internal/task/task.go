package task

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/idelchi/aura/internal/config/inherit"
	"github.com/idelchi/aura/internal/tmpl"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/aura/pkg/yamlutil"
	"github.com/idelchi/godyl/pkg/path/files"

	"go.yaml.in/yaml/v4"
)

// ForEach defines the iteration source for task commands.
type ForEach struct {
	File            string `yaml:"file"`              // Read lines from this file
	Shell           string `yaml:"shell"`             // Run command, read lines from stdout
	ContinueOnError bool   `yaml:"continue_on_error"` // Log per-item errors and continue instead of aborting
	Retries         int    `yaml:"retries"`           // Additional attempts per failed item (0 = no retry)
}

// taskDef is the raw YAML shape.
type taskDef struct {
	Description string            `yaml:"description"`
	Schedule    string            `yaml:"schedule"`
	Timeout     *time.Duration    `yaml:"timeout"`
	Disabled    *bool             `yaml:"disabled"`
	Agent       string            `yaml:"agent"`
	Mode        string            `yaml:"mode"`
	Session     string            `yaml:"session"`
	Workdir     string            `yaml:"workdir"`
	Tools       tool.Filter       `yaml:"tools"`
	Features    yaml.Node         `yaml:"features"` // raw YAML, decoded by consumer into config.Features
	Pre         []string          `yaml:"pre"`
	Commands    []string          `yaml:"commands"`
	Post        []string          `yaml:"post"`
	ForEach     *ForEach          `yaml:"foreach"`
	Finally     []string          `yaml:"finally"`
	OnMaxSteps  []string          `yaml:"on_max_steps"`
	Vars        map[string]string `yaml:"vars"`
	Env         map[string]string `yaml:"env"`
	EnvFile     []string          `yaml:"env_file"`
	Inherit     []string          `yaml:"inherit"`
}

// Task is a named scheduled task definition.
type Task struct {
	Name        string
	Inherit     []string // parent names (for display)
	Description string
	Schedule    string
	Timeout     time.Duration
	Disabled    *bool
	Agent       string
	Mode        string
	Session     string
	Workdir     string
	Tools       tool.Filter
	Features    yaml.Node // raw YAML for feature overrides, decoded by consumer into config.Features
	Pre         []string
	Commands    []string
	Post        []string
	ForEach     *ForEach
	Finally     []string
	OnMaxSteps  []string
	Vars        map[string]string
	Env         map[string]string
	EnvFile     []string
	Source      string // file path this task was loaded from
}

// IsManualOnly returns true when no schedule is configured (run via CLI only).
func (t Task) IsManualOnly() bool {
	return t.Schedule == ""
}

// IsDisabled returns true if the task is explicitly disabled.
func (t Task) IsDisabled() bool { return t.Disabled != nil && *t.Disabled }

// Validate checks the task definition for required fields.
func (t Task) Validate() error {
	if len(t.Commands) == 0 {
		return fmt.Errorf("task %q: commands is required and must not be empty", t.Name)
	}

	if t.ForEach != nil {
		if t.ForEach.File == "" && t.ForEach.Shell == "" {
			return fmt.Errorf("task %q: foreach requires either file or shell", t.Name)
		}

		if t.ForEach.File != "" && t.ForEach.Shell != "" {
			return fmt.Errorf("task %q: foreach cannot have both file and shell", t.Name)
		}
	}

	if t.ForEach != nil && t.ForEach.Retries < 0 {
		return fmt.Errorf("task %q: foreach.retries must be >= 0", t.Name)
	}

	if len(t.Finally) > 0 && t.ForEach == nil {
		return fmt.Errorf("task %q: finally requires foreach", t.Name)
	}

	return nil
}

// StatusDisplay returns "enabled" or "disabled".
func (t Task) StatusDisplay() string {
	if t.IsDisabled() {
		return "disabled"
	}

	return "enabled"
}

// ScheduleDisplay returns the schedule string, or "manual" if unset.
func (t Task) ScheduleDisplay() string {
	if t.Schedule == "" {
		return "manual"
	}

	return t.Schedule
}

// Display renders a table of all tasks with dynamic column widths and a header separator.
func (ts Tasks) Display() string {
	names := ts.Names()

	// Column headers.
	headers := [3]string{"NAME", "STATUS", "SCHEDULE"}

	// Start with header widths as minimums.
	widths := [3]int{len(headers[0]), len(headers[1]), len(headers[2])}

	// Scan data for max widths.
	for _, name := range names {
		t := ts[name]
		cols := [3]string{t.Name, t.StatusDisplay(), t.ScheduleDisplay()}

		for i, col := range cols {
			if len(col) > widths[i] {
				widths[i] = len(col)
			}
		}
	}

	// Build format string with two-space gaps.
	format := fmt.Sprintf("%%-%ds  %%-%ds  %%s\n", widths[0], widths[1])

	var b strings.Builder

	// Header.
	fmt.Fprintf(&b, format, headers[0], headers[1], headers[2])

	// Separator.
	fmt.Fprintf(&b, format,
		strings.Repeat("-", widths[0]),
		strings.Repeat("-", widths[1]),
		strings.Repeat("-", widths[2]),
	)

	// Rows.
	for _, name := range names {
		t := ts[name]
		fmt.Fprintf(&b, format, t.Name, t.StatusDisplay(), t.ScheduleDisplay())
	}

	return strings.TrimRight(b.String(), "\n")
}

func (t *Task) ApplyDefaults() {
	if t.Timeout == 0 {
		t.Timeout = 5 * time.Minute
	}
}

// Tasks maps task names to their definitions.
type Tasks map[string]Task

// Get returns a task by name, or nil if not found.
func (ts Tasks) Get(name string) *Task {
	t, ok := ts[name]
	if !ok {
		return nil
	}

	return &t
}

// Names returns a sorted slice of task names.
func (ts Tasks) Names() []string {
	names := make([]string, 0, len(ts))
	for name := range ts {
		names = append(names, name)
	}

	slices.Sort(names)

	return names
}

// Scheduled returns only tasks that have a schedule and are enabled.
func (ts Tasks) Scheduled() Tasks {
	result := make(Tasks)

	for name, t := range ts {
		if !t.IsDisabled() && !t.IsManualOnly() {
			result[name] = t
		}
	}

	return result
}

// Load reads YAML files and merges all task definitions into the map.
// Each file contains a map of task names to their definitions.
// Supports config inheritance via the `inherit` field within task definitions.
//
// Task files use two delimiter systems:
//   - {{ }} for load-time templates (structural generation, env vars, --set vars, sprig)
//   - $[[ ]] for runtime templates (per-command vars like .Item, .Workdir)
//
// Load-time templates are expanded before YAML parsing (Phase A). After parsing,
// convertRuntimeDelimiters replaces $[[ ... ]] → {{ ... }} on command-like fields
// so the runtime template engine sees standard {{ }} with full sprig support.
//
// Command-like fields (pre, commands, post, env, foreach, finally, on_max_steps) are
// preserved from the initial parse so runtime template expressions survive
// Phase C's metadata expansion.
func (ts *Tasks) Load(ff files.Files, vars map[string]string) error {
	*ts = make(Tasks)

	// Phase A: Expand load-time templates, parse YAML, convert runtime delimiters.
	all := make(map[string]taskDef)
	sourceMap := make(map[string]string)

	for _, f := range ff {
		content, err := f.Read()
		if err != nil {
			return fmt.Errorf("reading task definition: %w", err)
		}

		expanded, err := tmpl.Expand(content, vars)
		if err != nil {
			return fmt.Errorf("expanding task file %s: %w", f, err)
		}

		var fileTasks map[string]taskDef
		if err := yaml.Unmarshal(expanded, &fileTasks); err != nil {
			return fmt.Errorf("parsing task file %s: %w", f, err)
		}

		for name, def := range fileTasks {
			convertRuntimeDelimiters(&def)

			all[name] = def
			sourceMap[name] = f.Path()
		}
	}

	// Phase B: Resolve inheritance (struct-level merge).
	resolved, err := inherit.Resolve(all, func(d taskDef) []string {
		return d.Inherit
	})
	if err != nil {
		return err
	}

	// Phase C: Two-pass decode from merged structs.
	for name, merged := range resolved {
		// Save raw command fields and Features before template expansion.
		// These must survive unexpanded (command fields have runtime templates,
		// yaml.Node does not survive marshal→unmarshal round-trip).
		rawPre := merged.Pre
		rawCommands := merged.Commands
		rawPost := merged.Post
		rawForEach := merged.ForEach
		rawFinally := merged.Finally
		rawOnMaxSteps := merged.OnMaxSteps
		rawVars := merged.Vars
		rawEnv := merged.Env
		rawFeatures := merged.Features

		// Expanded decode: marshal merged struct → expand templates → strict unmarshal.
		// Task vars provide defaults; --set vars (the `vars` parameter) override.
		expandVars := maps.Clone(merged.Vars)
		if expandVars == nil {
			expandVars = make(map[string]string)
		}

		maps.Copy(expandVars, vars)

		mergedBytes, err := yaml.Marshal(map[string]taskDef{name: merged})
		if err != nil {
			return fmt.Errorf("task %q: re-marshaling merged struct: %w", name, err)
		}

		expanded, err := tmpl.Expand(mergedBytes, expandVars)
		if err != nil {
			return fmt.Errorf("expanding task template %q: %w", name, err)
		}

		var file map[string]taskDef
		if err := yamlutil.StrictUnmarshal(expanded, &file); err != nil {
			return fmt.Errorf("parsing task definition %q: %w", name, err)
		}

		def, ok := file[name]
		if !ok {
			return fmt.Errorf("task %q: not found after expanded parse", name)
		}

		// Resolve timeout from pointer.
		var timeout time.Duration

		if def.Timeout != nil {
			timeout = *def.Timeout
		}

		t := Task{
			// Metadata from expanded parse.
			Name:        name,
			Inherit:     merged.Inherit,
			Description: def.Description,
			Schedule:    def.Schedule,
			Timeout:     timeout,
			Disabled:    def.Disabled,
			Agent:       def.Agent,
			Mode:        def.Mode,
			Session:     def.Session,
			Workdir:     def.Workdir,
			Tools:       def.Tools,
			// Features and command fields from raw (pre-expansion) merge.
			Features:   rawFeatures,
			Pre:        rawPre,
			Commands:   rawCommands,
			Post:       rawPost,
			ForEach:    rawForEach,
			Finally:    rawFinally,
			OnMaxSteps: rawOnMaxSteps,
			Vars:       rawVars,     // command-like: preserved raw for runtime expansion
			Env:        rawEnv,      // command-like: preserved raw for runtime expansion
			EnvFile:    def.EnvFile, // metadata: from expanded parse
		}

		t.Source = sourceMap[name]

		t.ApplyDefaults()

		if err := t.Validate(); err != nil {
			return err
		}

		(*ts)[name] = t
	}

	return nil
}

// runtimeDelimRe matches $[[ ... ]] pairs and captures the inner expression.
// Lazy (.+?) ensures each $[[ matches the nearest ]], leaving bare bash ]] untouched.
var runtimeDelimRe = regexp.MustCompile(`\$\[\[(.+?)\]\]`)

// convertRuntimeDelimiters replaces $[[ ... ]] → {{ ... }} on all command-like
// fields in a taskDef. This converts runtime template expressions from the
// user-facing $[[ ]] syntax to standard Go template delimiters for execution.
func convertRuntimeDelimiters(def *taskDef) {
	replace := func(s string) string {
		return runtimeDelimRe.ReplaceAllString(s, "{{$1}}")
	}

	replaceSlice := func(ss []string) {
		for i, s := range ss {
			ss[i] = replace(s)
		}
	}

	replaceMap := func(m map[string]string) {
		for k, v := range m {
			m[k] = replace(v)
		}
	}

	replaceSlice(def.Pre)
	replaceSlice(def.Commands)
	replaceSlice(def.Post)
	replaceSlice(def.Finally)
	replaceSlice(def.OnMaxSteps)
	replaceMap(def.Env)

	if def.ForEach != nil {
		def.ForEach.File = replace(def.ForEach.File)
		def.ForEach.Shell = replace(def.ForEach.Shell)
	}
}
