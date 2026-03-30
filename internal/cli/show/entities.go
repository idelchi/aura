package show

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	cli "github.com/urfave/cli/v3"

	"github.com/idelchi/aura/internal/cli/core"
	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/debug"
	pluginspkg "github.com/idelchi/aura/internal/plugins"
	"github.com/idelchi/aura/pkg/filter"
	"github.com/idelchi/aura/pkg/gitutil"
	"github.com/idelchi/aura/sdk/version"
	"github.com/idelchi/godyl/pkg/path/folder"
	"github.com/idelchi/godyl/pkg/pretty"
)

func agentsCommand(flags *core.Flags) *cli.Command {
	return entityCommand("agents", "List or inspect agents", flags,
		[]config.Part{config.PartAgents},
		func(cfg *config.Config, filters []string) error {
			t := newTable("NAME", "MODE", "MODEL", "DESCRIPTION")

			for _, a := range cfg.Agents.Values() {
				ok, err := filter.Match(a.Metadata, filters)
				if err != nil {
					return err
				}

				if ok {
					t.add(a.Name(), a.Metadata.Mode, a.Metadata.Model.Name, a.Metadata.Description)
				}
			}

			t.print()

			return nil
		},
		func(cfg *config.Config, name string) error {
			a := cfg.Agents.Get(name)
			if a == nil {
				return fmt.Errorf("agent %q not found", name)
			}

			m := a.Metadata
			d := newDetail(12)
			d.Field("Name", m.Name)
			d.Field("Description", m.Description)
			d.Field("Model", m.Model.Name)
			d.Field("Provider", m.Model.Provider)

			if think := m.Model.Think.String(); think != "" && think != "off" {
				d.Field("Thinking", think)
			}

			if m.Model.Context > 0 {
				d.Field("Context", strconv.Itoa(m.Model.Context))
			}

			d.Field("Mode", m.Mode)
			d.Field("System", m.System)
			d.BoolField("Default", m.Default)
			d.BoolField("Hidden", m.Hide)
			d.BoolField("Subagent", m.Subagent)

			if len(m.Inherit) > 0 {
				d.Field("Inherit", strings.Join(m.Inherit, ", "))
			}

			if len(m.Fallback) > 0 {
				d.Field("Fallback", strings.Join(m.Fallback, ", "))
			}

			fmt.Println(d)

			return nil
		},
	)
}

func modesCommand(flags *core.Flags) *cli.Command {
	return entityCommand("modes", "List or inspect modes", flags,
		[]config.Part{config.PartModes},
		func(cfg *config.Config, filters []string) error {
			t := newTable("NAME", "DESCRIPTION")

			for _, m := range cfg.Modes.Values() {
				ok, err := filter.Match(m.Metadata, filters)
				if err != nil {
					return err
				}

				if ok {
					t.add(m.Name(), m.Metadata.Description)
				}
			}

			t.print()

			return nil
		},
		func(cfg *config.Config, name string) error {
			m := cfg.Modes.Get(name)
			if m == nil {
				return fmt.Errorf("mode %q not found", name)
			}

			md := m.Metadata
			d := newDetail(12)
			d.Field("Name", md.Name)
			d.Field("Description", md.Description)
			d.BoolField("Hidden", md.Hide)

			if len(md.Tools.Enabled) > 0 {
				d.Field("Tools", strings.Join(md.Tools.Enabled, ", "))
			}

			if len(md.Tools.Disabled) > 0 {
				d.Field("Disabled", strings.Join(md.Tools.Disabled, ", "))
			}

			if len(md.Inherit) > 0 {
				d.Field("Inherit", strings.Join(md.Inherit, ", "))
			}

			d.Body(string(m.Prompt))
			fmt.Println(d)

			return nil
		},
	)
}

func promptsCommand(flags *core.Flags) *cli.Command {
	return entityCommand("prompts", "List or inspect system prompts", flags,
		[]config.Part{config.PartSystems},
		func(cfg *config.Config, filters []string) error {
			t := newTable("NAME", "DESCRIPTION")

			for _, s := range cfg.Systems.Values() {
				ok, err := filter.Match(s.Metadata, filters)
				if err != nil {
					return err
				}

				if ok {
					t.add(s.Name(), s.Metadata.Description)
				}
			}

			t.print()

			return nil
		},
		func(cfg *config.Config, name string) error {
			s := cfg.Systems.Get(name)
			if s == nil {
				return fmt.Errorf("prompt %q not found", name)
			}

			d := newDetail(12)
			d.Field("Name", s.Metadata.Name)
			d.Field("Description", s.Metadata.Description)

			if len(s.Metadata.Inherit) > 0 {
				d.Field("Inherit", strings.Join(s.Metadata.Inherit, ", "))
			}

			d.Body(string(s.Prompt))
			fmt.Println(d)

			return nil
		},
	)
}

func providersCommand(flags *core.Flags) *cli.Command {
	return entityCommand("providers", "List or inspect providers", flags,
		[]config.Part{config.PartProviders},
		func(cfg *config.Config, filters []string) error {
			t := newTable("NAME", "TYPE", "URL", "CAPABILITIES")

			for _, name := range cfg.Providers.Names() {
				p := cfg.Providers[name]

				ok, err := filter.Match(p, filters)
				if err != nil {
					return err
				}

				if !ok {
					continue
				}

				caps := make([]string, 0, len(p.Capabilities))
				for _, c := range p.Capabilities {
					caps = append(caps, string(c))
				}

				capsStr := strings.Join(caps, ", ")
				if capsStr == "" {
					capsStr = "all"
				}

				t.add(name, p.Type, p.URL, capsStr)
			}

			t.print()

			return nil
		},
		func(cfg *config.Config, name string) error {
			p := cfg.Providers.Get(name)
			if p == nil {
				return fmt.Errorf("provider %q not found", name)
			}

			d := newDetail(14)
			d.Field("Type", p.Type)
			d.Field("URL", p.URL)
			d.Field("Timeout", p.Timeout)
			d.Field("KeepAlive", p.KeepAlive)

			caps := make([]string, 0, len(p.Capabilities))
			for _, c := range p.Capabilities {
				caps = append(caps, string(c))
			}

			capsStr := strings.Join(caps, ", ")
			if capsStr == "" {
				capsStr = "all"
			}

			d.Field("Capabilities", capsStr)

			if len(p.Models.Include) > 0 {
				d.Field("Models.Include", strings.Join(p.Models.Include, ", "))
			}

			if len(p.Models.Exclude) > 0 {
				d.Field("Models.Exclude", strings.Join(p.Models.Exclude, ", "))
			}

			if p.Retry.MaxAttempts > 0 {
				d.Field(
					"Retry",
					fmt.Sprintf("max=%d base=%s max=%s", p.Retry.MaxAttempts, p.Retry.BaseDelay, p.Retry.MaxDelay),
				)
			}

			fmt.Println(d)

			return nil
		},
	)
}

func hooksCommand(flags *core.Flags) *cli.Command {
	return entityCommand("hooks", "List or inspect hooks", flags,
		[]config.Part{config.PartHooks},
		func(cfg *config.Config, filters []string) error {
			t := newTable("NAME", "EVENT", "STATUS", "DESCRIPTION")
			names := slices.Sorted(maps.Keys(cfg.Hooks))

			for _, name := range names {
				h := cfg.Hooks[name]

				ok, err := filter.Match(h, filters)
				if err != nil {
					return err
				}

				if !ok {
					continue
				}

				status := "enabled"

				if h.IsDisabled() {
					status = "disabled"
				}

				t.add(name, h.Event, status, h.Description)
			}

			t.print()

			return nil
		},
		func(cfg *config.Config, name string) error {
			h, ok := cfg.Hooks[name]
			if !ok {
				return fmt.Errorf("hook %q not found", name)
			}

			d := newDetail(12)
			d.Field("Description", h.Description)
			d.Field("Event", h.Event)
			d.Field("Matcher", h.Matcher)
			d.Field("Files", h.Files)
			d.Field("Command", h.Command)

			if h.TimeoutSeconds() > 0 {
				d.Field("Timeout", fmt.Sprintf("%ds", h.TimeoutSeconds()))
			}

			if len(h.Depends) > 0 {
				d.Field("Depends", strings.Join(h.Depends, ", "))
			}

			d.BoolField("Silent", h.Silent)
			d.BoolField("Disabled", h.Disabled)

			if len(h.Inherit) > 0 {
				d.Field("Inherit", strings.Join(h.Inherit, ", "))
			}

			fmt.Println(d)

			return nil
		},
	)
}

func featuresCommand(flags *core.Flags) *cli.Command {
	return entityCommand("features", "Show feature configuration", flags,
		[]config.Part{config.PartFeatures},
		func(cfg *config.Config, _ []string) error {
			fmt.Print(pretty.YAML(cfg.Features))

			return nil
		},
		func(cfg *config.Config, _ string) error {
			fmt.Print(pretty.YAML(cfg.Features))

			return nil
		},
	)
}

func pluginsCommand(flags *core.Flags) *cli.Command {
	return entityCommand("plugins", "List or inspect plugins", flags,
		[]config.Part{config.PartPlugins},
		func(cfg *config.Config, filters []string) error {
			t := newTable("NAME", "STATUS", "DESCRIPTION")

			for _, name := range cfg.Plugins.Names() {
				p := cfg.Plugins[name]

				ok, err := filter.Match(p, filters)
				if err != nil {
					return err
				}

				if !ok {
					continue
				}

				status := "enabled"

				if !p.IsEnabled() {
					status = "disabled"
				}

				t.add(name, status, p.Description)
			}

			t.print()

			return nil
		},
		func(cfg *config.Config, name string) error {
			p := cfg.Plugins.Get(name)
			if p == nil {
				return fmt.Errorf("plugin %q not found", name)
			}

			d := newDetail(12)
			d.Field("Name", p.Name)
			d.Field("Description", p.Description)

			// SDK version and compatibility check via Yaegi probe.
			caps, probeErr := pluginspkg.ProbeCapabilities(p.Dir())
			if probeErr != nil {
				debug.Log("[show] plugin probe %s: %v", p.Name, probeErr)
				d.Field("Probe", fmt.Sprintf("(failed: %v)", probeErr))
			}

			if caps.SDKVersion != "" {
				if err := pluginspkg.IsSDKCompatible(caps.SDKVersion, version.Version); err == nil {
					d.Field("SDK Version", caps.SDKVersion+" (compatible)")
				} else {
					d.Field(
						"SDK Version",
						fmt.Sprintf("%s (INCOMPATIBLE — host has %s)", caps.SDKVersion, version.Version),
					)
				}
			}

			status := "enabled"

			if !p.IsEnabled() {
				status = "disabled"
			}

			d.Field("Status", status)
			d.Field("Condition", p.Condition)
			d.Field("Source", p.Source)

			if p.HasOrigin() {
				d.Field("Origin", p.Origin.URL)
				d.Field("Ref", p.Origin.Ref)
				d.Field("Commit", p.Origin.Commit)

				if len(p.Origin.Subpaths) > 0 {
					d.Field("Subpaths", strings.Join(p.Origin.Subpaths, ", "))
				}
			}

			if probeErr == nil {
				if len(caps.Hooks) > 0 {
					d.Field("Hooks", strings.Join(caps.Hooks, ", "))
				}

				if caps.ToolName != "" {
					d.Field("Tool", caps.ToolName)
				}

				if caps.CommandName != "" {
					d.Field("Command", "/"+caps.CommandName)
				}
			}

			allFiles, err := folder.New(p.Dir()).ListFiles()
			if err == nil && len(allFiles) > 0 {
				d.Blank()

				names := make([]string, len(allFiles))
				for i, f := range allFiles {
					names[i] = f.Base()
				}

				d.SliceField("Files", names)
			}

			fmt.Println(d)

			return nil
		},
	)
}

func skillsCommand(flags *core.Flags) *cli.Command {
	return entityCommand("skills", "List or inspect skills", flags,
		[]config.Part{config.PartSkills},
		func(cfg *config.Config, filters []string) error {
			t := newTable("NAME", "DESCRIPTION")

			for _, s := range cfg.Skills.Values() {
				ok, err := filter.Match(s, filters)
				if err != nil {
					return err
				}

				if ok {
					t.add(s.Name(), s.Metadata.Description)
				}
			}

			t.print()

			return nil
		},
		func(cfg *config.Config, name string) error {
			f, s := cfg.Skills.GetWithKey(name)
			if s == nil {
				return fmt.Errorf("skill %q not found", name)
			}

			d := newDetail(12)
			d.Field("Name", s.Metadata.Name)
			d.Field("Description", s.Metadata.Description)
			d.Field("Source", f.Path())

			dir := f.Dir()
			if origin, _, found := gitutil.FindOrigin(dir); found {
				d.Field("Origin", origin.URL)
				d.Field("Ref", origin.Ref)
				d.Field("Commit", origin.Commit)

				if len(origin.Subpaths) > 0 {
					d.Field("Subpaths", strings.Join(origin.Subpaths, ", "))
				}
			}

			d.Body(s.Body)
			fmt.Println(d)

			return nil
		},
	)
}

func tasksCommand(flags *core.Flags) *cli.Command {
	return entityCommand("tasks", "List or inspect tasks", flags,
		[]config.Part{config.PartTasks},
		func(cfg *config.Config, filters []string) error {
			t := newTable("NAME", "STATUS", "SCHEDULE", "DESCRIPTION")
			names := slices.Sorted(func(yield func(string) bool) {
				for name := range cfg.Tasks {
					if !yield(name) {
						return
					}
				}
			})

			for _, name := range names {
				task := cfg.Tasks[name]

				ok, err := filter.Match(task, filters)
				if err != nil {
					return err
				}

				if !ok {
					continue
				}

				t.add(name, task.StatusDisplay(), task.ScheduleDisplay(), task.Description)
			}

			t.print()

			return nil
		},
		func(cfg *config.Config, name string) error {
			t := cfg.Tasks.Get(name)
			if t == nil {
				return fmt.Errorf("task %q not found", name)
			}

			d := newDetail(12)
			d.Field("Name", t.Name)
			d.Field("Description", t.Description)
			d.Field("Schedule", t.ScheduleDisplay())
			d.Field("Timeout", t.Timeout.String())
			d.Field("Status", t.StatusDisplay())
			d.Field("Agent", t.Agent)
			d.Field("Mode", t.Mode)
			d.Field("Session", t.Session)
			d.Field("Workdir", t.Workdir)

			if t.Tools.IsSet() {
				if len(t.Tools.Enabled) > 0 {
					d.Field("Tools", strings.Join(t.Tools.Enabled, ", "))
				}

				if len(t.Tools.Disabled) > 0 {
					d.Field("Tools.Off", strings.Join(t.Tools.Disabled, ", "))
				}
			}

			d.SliceField("Pre", t.Pre)

			if t.ForEach != nil {
				var feVal string

				if t.ForEach.File != "" {
					feVal = "file: " + t.ForEach.File
				} else {
					feVal = "shell: " + t.ForEach.Shell
				}

				if t.ForEach.ContinueOnError {
					feVal += " (continue_on_error)"
				}

				d.Field("ForEach", feVal)
			}

			d.SliceField("Commands", t.Commands)
			d.SliceField("Finally", t.Finally)
			d.SliceField("OnMaxSteps", t.OnMaxSteps)
			d.SliceField("Post", t.Post)

			fmt.Println(d)

			return nil
		},
	)
}
