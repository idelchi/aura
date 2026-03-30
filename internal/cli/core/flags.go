package core

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/mirror"
)

// NotSet is the default IsSet function — always returns false.
func NotSet(string) bool { return false }

// Flags holds all CLI flag values, resolved via urfave/cli Destination pointers.
// Persistent (root) flags are top-level fields; subcommand flags
// are nested in per-command sub-structs.
// Precedence: CLI flag > AURA_* env var > default value.
type Flags struct {
	Config         []string
	Provider       string
	Agent          string
	Model          string
	EnvFile        []string
	Workdir        string
	Show           bool
	Print          bool
	PrintEnv       bool
	Simple         bool
	Auto           bool
	Mode           string
	System         string
	Think          string
	Debug          bool
	Experiments    bool
	WithoutPlugins bool
	UnsafePlugins  bool
	Resume         string
	Continue       bool
	Output         string
	IncludeTools   []string
	ExcludeTools   []string
	IncludeMCPs    []string
	ExcludeMCPs    []string
	MaxSteps       int
	TokenBudget    int
	Providers      []string
	Dry            string
	NoCache        bool
	Override       []string

	Set    map[string]string
	Home   string
	Mirror *mirror.Mirror
	Writer io.Writer

	// Subcommand flags (nested by command path).
	Run        RunFlags
	Web        WebFlags
	Models     ModelsFlags
	Tools      ToolsFlags
	Query      QueryFlags
	Init       InitFlags
	Transcribe TranscribeFlags
	Speak      SpeakFlags
	Tasks      TasksFlags
	Plugins    PluginsFlags
	Skills     SkillsFlags
	Login      LoginFlags
	Tokens     TokensFlags
	ShowCmd    ShowCmdFlags

	// IsSet reports whether a root-level flag was explicitly set via CLI or env var.
	// Wired to cmd.IsSet in the Before hook.
	IsSet func(string) bool
}

// ── Subcommand flag structs ──────────────────────────────────────────────

type RunFlags struct {
	Timeout time.Duration
}

type WebFlags struct {
	Bind string
}

type ModelsFlags struct {
	SortBy string
	Filter []string
	Name   []string
}

type ToolsFlags struct {
	JSON       bool
	Raw        bool
	RO         bool
	ROPaths    []string
	RWPaths    []string
	Headless   bool
	WithMCP    bool
	MCPServers []string
}

type QueryFlags struct {
	Top int
}

type InitFlags struct {
	Dir string
}

type TranscribeFlags struct {
	Language string
}

type SpeakFlags struct {
	Voice string
}

type TasksFlags struct {
	Files []string
	Run   TasksRunFlags
}

type TasksRunFlags struct {
	Prepend     []string
	Append      []string
	Start       int
	Timeout     time.Duration
	Now         bool
	Concurrency uint

	// IsSet reports whether a tasks-run-level flag was explicitly set.
	IsSet func(string) bool
}

type PluginsFlags struct {
	Add    PluginsAddFlags
	Update PluginsUpdateFlags
}

type PluginsAddFlags struct {
	Name     string
	Global   bool
	Ref      string
	NoVendor bool
	Subpath  []string
}

type PluginsUpdateFlags struct {
	All      bool
	NoVendor bool
}

type SkillsFlags struct {
	Add    SkillsAddFlags
	Update SkillsUpdateFlags
}

type SkillsAddFlags struct {
	Name    string
	Global  bool
	Ref     string
	Subpath []string
}

type SkillsUpdateFlags struct {
	All bool
}

type LoginFlags struct {
	Local bool
}

type TokensFlags struct {
	Method []string
}

type ShowCmdFlags struct {
	Filter []string
}

// ── Flag storage ─────────────────────────────────────────────────────────

var parsedFlags Flags

// StoreFlags saves the parsed flags for later retrieval via GetFlags.
// Called once from the root Before hook after Destination pointers are resolved.
func StoreFlags(f Flags) { parsedFlags = f }

// GetFlags returns the flags stored during the root Before hook.
func GetFlags() Flags { return parsedFlags }

// ── Methods ──────────────────────────────────────────────────────────────

// LoadEnvFiles loads environment files from the EnvFile list.
// When explicit is true (CLI flag or AURA_ENV_FILE), every file must exist.
// When false (default), missing files are silently ignored.
func (f Flags) LoadEnvFiles(explicit bool) error {
	if explicit {
		for _, path := range f.EnvFile {
			if err := godotenv.Load(path); err != nil {
				return fmt.Errorf("loading env file %q: %w", path, err)
			}
		}

		return nil
	}

	godotenv.Load(f.EnvFile...)

	return nil
}

// ToEnv returns AURA_KEY=value pairs for every flag explicitly set via CLI or env var.
func (f Flags) ToEnv() []string {
	type entry struct {
		name   string
		envKey string
		value  func() string
	}

	fmtBool := func(b bool) string {
		if b {
			return "true"
		}

		return "false"
	}

	entries := []entry{
		{"home", "AURA_HOME", func() string { return f.Home }},
		{"config", "AURA_CONFIG", func() string { return strings.Join(f.Config, ",") }},
		{"agent", "AURA_AGENT", func() string { return f.Agent }},
		{"provider", "AURA_PROVIDER", func() string { return f.Provider }},
		{"mode", "AURA_MODE", func() string { return f.Mode }},
		{"system", "AURA_SYSTEM", func() string { return f.System }},
		{"think", "AURA_THINK", func() string { return f.Think }},
		{"model", "AURA_MODEL", func() string { return f.Model }},
		{"env-file", "AURA_ENV_FILE", func() string { return strings.Join(f.EnvFile, ",") }},
		{"show", "AURA_SHOW", func() string { return fmtBool(f.Show) }},
		{"print", "AURA_PRINT", func() string { return fmtBool(f.Print) }},
		{"print-env", "AURA_PRINT_ENV", func() string { return fmtBool(f.PrintEnv) }},
		{"simple", "AURA_SIMPLE", func() string { return fmtBool(f.Simple) }},
		{"auto", "AURA_AUTO", func() string { return fmtBool(f.Auto) }},
		{"debug", "AURA_DEBUG", func() string { return fmtBool(f.Debug) }},
		{"experiments", "AURA_EXPERIMENTS", func() string { return fmtBool(f.Experiments) }},
		{"without-plugins", "AURA_WITHOUT_PLUGINS", func() string { return fmtBool(f.WithoutPlugins) }},
		{"unsafe-plugins", "AURA_UNSAFE_PLUGINS", func() string { return fmtBool(f.UnsafePlugins) }},
		{"include-tools", "AURA_INCLUDE_TOOLS", func() string { return strings.Join(f.IncludeTools, ",") }},
		{"exclude-tools", "AURA_EXCLUDE_TOOLS", func() string { return strings.Join(f.ExcludeTools, ",") }},
		{"include-mcps", "AURA_INCLUDE_MCPS", func() string { return strings.Join(f.IncludeMCPs, ",") }},
		{"exclude-mcps", "AURA_EXCLUDE_MCPS", func() string { return strings.Join(f.ExcludeMCPs, ",") }},
		{"max-steps", "AURA_MAX_STEPS", func() string { return strconv.Itoa(f.MaxSteps) }},
		{"token-budget", "AURA_TOKEN_BUDGET", func() string { return strconv.Itoa(f.TokenBudget) }},
		{"workdir", "AURA_WORKDIR", func() string { return f.Workdir }},
		{"resume", "AURA_RESUME", func() string { return f.Resume }},
		{"continue", "AURA_CONTINUE", func() string { return fmtBool(f.Continue) }},
		{"output", "AURA_OUTPUT", func() string { return f.Output }},
		{"providers", "AURA_PROVIDERS", func() string { return strings.Join(f.Providers, ",") }},
		{"dry", "AURA_DRY", func() string { return f.Dry }},
		{"override", "AURA_OVERRIDE", func() string { return strings.Join(f.Override, ",") }},
		{"set", "AURA_SET", func() string {
			pairs := make([]string, 0, len(f.Set))
			for k, v := range f.Set {
				pairs = append(pairs, k+"="+v)
			}

			return strings.Join(pairs, ",")
		}},
	}

	var env []string

	for _, e := range entries {
		if f.IsSet(e.name) {
			env = append(env, e.envKey+"="+e.value())
		}
	}

	return env
}

// ConfigOptions returns the base config.Options derived from CLI flags.
// Callers that need additional fields (WithPlugins, ExtraTaskFiles, etc.)
// can mutate the returned struct before passing it to config.New.
func (f Flags) ConfigOptions() config.Options {
	return config.Options{
		Homes:      f.Homes(),
		WriteHome:  f.WriteHome(),
		GlobalHome: f.Home,
		SetVars:    f.Set,
	}
}

// WriteHome returns the primary config directory for writes (sessions, debug, auth, plugins).
// This is the last non-empty element of Config, falling back to Home (global config home).
func (f Flags) WriteHome() string {
	for i := len(f.Config) - 1; i >= 0; i-- {
		if f.Config[i] != "" {
			return f.Config[i]
		}
	}

	return f.Home
}

// Homes returns all config directories in merge order: global home first, then
// the --config entries. Files.Load iterates left-to-right with last-wins dedup.
func (f Flags) Homes() []string {
	return append([]string{f.Home}, f.Config...)
}
