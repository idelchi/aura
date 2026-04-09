package plugins

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/MakeNowJust/heredoc/v2"
	cli "github.com/urfave/cli/v3"

	"github.com/idelchi/aura/internal/cli/core"
	"github.com/idelchi/aura/internal/cli/packs"
	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/debug"
	pluginspkg "github.com/idelchi/aura/internal/plugins"
	"github.com/idelchi/aura/pkg/gitutil"
	"github.com/idelchi/aura/sdk/version"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/files"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// Command creates the 'aura plugins' command with add/update/remove subcommands.
func Command(flags *core.Flags) *cli.Command {
	return &cli.Command{
		Name:  "plugins",
		Usage: "Manage plugins",
		Description: heredoc.Doc(`
			Manage user-defined Go plugins.

			Plugins are directories of .go files in .aura/plugins/<name>/
			that hook into the conversation lifecycle via Yaegi.

			For repositories requiring authentication, the following environment variables are considered:

			- GIT_USERNAME/GIT_PASSWORD
			- GITHUB_TOKEN
			- GITLAB_TOKEN
			- GIT_TOKEN
		`),
		Before: func(_ context.Context, _ *cli.Command) (context.Context, error) {
			flags := core.GetFlags()
			debug.Init(flags.WriteHome(), flags.Debug)

			return nil, nil
		},
		After: func(_ context.Context, _ *cli.Command) error {
			debug.Close()

			return nil
		},
		Commands: []*cli.Command{
			add(flags),
			update(flags),
			remove(),
		},
	}
}

func add(flags *core.Flags) *cli.Command {
	return &cli.Command{
		Name:  "add",
		Usage: "Install plugins from git URLs or local paths",
		Description: heredoc.Doc(`
			Install plugins from git repositories or local directories.

			Git sources (HTTPS or SSH URLs) are cloned into the plugins directory.
			Local paths are copied into the plugins directory.

			The plugin name defaults to the last segment of the source URL
			(stripped of "aura-plugin-" prefix). Override with --name (single source only).

			Multiple sources can be provided when --name and --ref are not set.
		`),
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "name",
				Usage:       "Override the plugin directory name",
				Destination: &flags.Plugins.Add.Name,
			},
			&cli.BoolFlag{
				Name:        "global",
				Usage:       "Install to global plugins directory (~/.aura)",
				Destination: &flags.Plugins.Add.Global,
			},
			&cli.StringFlag{
				Name:        "ref",
				Usage:       "Git branch or tag to clone",
				Destination: &flags.Plugins.Add.Ref,
			},
			&cli.BoolFlag{
				Name:        "no-vendor",
				Usage:       "Skip go mod tidy and go mod vendor after installing",
				Destination: &flags.Plugins.Add.NoVendor,
			},
			&cli.StringSliceFlag{
				Name:        "subpath",
				Usage:       "Subdirectory within the source to use as plugin root (repeatable)",
				Destination: &flags.Plugins.Add.Subpath,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() < 1 {
				return errors.New("expected at least 1 argument")
			}

			flags := core.GetFlags()

			args := cmd.Args().Slice()

			if len(args) > 1 &&
				(flags.Plugins.Add.Name != "" || flags.Plugins.Add.Ref != "" || len(flags.Plugins.Add.Subpath) > 0) {
				return errors.New("--name, --ref, and --subpath cannot be used with multiple sources")
			}

			if flags.Plugins.Add.Name != "" && len(flags.Plugins.Add.Subpath) > 1 {
				return errors.New("--name cannot be used with multiple --subpath values")
			}

			subpaths, err := packs.CleanSubpaths(flags.Plugins.Add.Subpath)
			if err != nil {
				return err
			}

			pluginsHome := flags.WriteHome()
			if flags.Plugins.Add.Global {
				pluginsHome = flags.Home
			}

			if pluginsHome == "" {
				return errors.New("no config directory found (run 'aura init' first)")
			}

			pluginsDir := config.ResolvePluginDir(pluginsHome)
			vendor := !flags.Plugins.Add.NoVendor

			var errs []error

			for _, source := range args {
				if gitutil.IsGitURL(source) {
					err := addGit(
						cmd.Writer,
						source,
						pluginsDir,
						flags.Plugins.Add.Name,
						flags.Plugins.Add.Ref,
						subpaths,
						vendor,
					)
					if err != nil {
						errs = append(errs, err)
					}
				} else {
					// Local paths: use first subpath only (local copies are scoped to a single path).
					var subpath string

					if len(subpaths) > 0 {
						subpath = subpaths[0]
					}

					err := addLocal(cmd.Writer, source, pluginsDir, flags.Plugins.Add.Name, subpath, vendor)
					if err != nil {
						errs = append(errs, err)
					}
				}
			}

			if len(errs) > 0 {
				return fmt.Errorf("adding %d source(s):\n%w", len(errs), errors.Join(errs...))
			}

			return nil
		},
	}
}

func addGit(w io.Writer, source, pluginsDir, name, ref string, subpaths []string, vendor bool) error {
	if name == "" {
		name = packs.DeriveName(source, subpaths, "aura-plugin-")
	}

	result, err := packs.CloneSource(w, source, pluginsDir, name, ref, subpaths, "plugin")
	if err != nil {
		return err
	}

	// Discover plugins across all subpath roots (or entire clone if none).
	var found files.Files

	for _, root := range packs.DiscoveryRoots(result.TargetDir, subpaths) {
		discovered, err := config.DiscoverPlugins(folder.New(root))
		if err != nil {
			debug.Log("[plugins:add] discover in %s: %v", root, err)
		}

		found = append(found, discovered...)
	}

	if len(found) == 0 {
		folder.New(result.TargetDir).Remove()

		return errors.New("no plugin.yaml found — not a valid aura plugin or plugin pack")
	}

	debug.Log("[plugins:add] discovered %d plugin(s)", len(found))

	// Vendor dependencies if requested — each plugin dir independently.
	if vendor {
		for _, f := range found {
			pluginDir := folder.FromFile(f).Path()

			debug.Log("[plugins:add] vendoring %s", pluginDir)
			fmt.Fprintf(w, "Vendoring %s...\n", pluginDir)

			if err := pluginspkg.Vendor(pluginDir); err != nil {
				folder.New(result.TargetDir).Remove()

				return fmt.Errorf("vendoring %s: %w", pluginDir, err)
			}
		}
	}

	// Verify SDK compatibility (runs even with --no-vendor: cloned repos may ship vendor/).
	for _, f := range found {
		pluginDir := folder.FromFile(f).Path()

		if err := checkSDKCompat(pluginDir, name); err != nil {
			folder.New(result.TargetDir).Remove()

			return err
		}
	}

	debug.Log("[plugins:add] SDK compatibility OK")

	packs.WriteCloneOrigin(w, result.TargetDir, source, ref, result.Commit, subpaths)

	fmt.Fprintf(w, "Installed %d plugin(s) from %q\n", len(found), name)

	for _, f := range found {
		pluginDir := folder.FromFile(f).Path()
		pluginName := folder.FromFile(f).Base()

		caps, probeErr := pluginspkg.ProbeCapabilities(pluginDir)
		if probeErr != nil {
			debug.Log("[plugin] probe %s: %v", pluginName, probeErr)

			fmt.Fprintf(w, "  %s (probe failed)\n", pluginName)
		} else {
			fmt.Fprintf(w, "  %s%s\n", pluginName, formatCapsSummary(caps))
		}
	}

	if result.Commit != "" {
		fmt.Fprintf(w, "Commit: %s\n", gitutil.ShortCommit(result.Commit))
	}

	return nil
}

func addLocal(w io.Writer, source, pluginsDir, name, subpath string, vendor bool) error {
	srcFolder := folder.New(source)
	if !srcFolder.Exists() {
		return fmt.Errorf("source directory %q does not exist", source)
	}

	// Pre-copy: validate source has plugins (scoped by subpath).
	effectiveSource := source

	if subpath != "" {
		effectiveSource = folder.New(source, subpath).Path()

		if !folder.New(effectiveSource).Exists() {
			return fmt.Errorf("subpath %q does not exist in %s", subpath, source)
		}
	}

	found, err := config.DiscoverPlugins(folder.New(effectiveSource))
	if err != nil || len(found) == 0 {
		return errors.New("no plugin.yaml found — not a valid aura plugin or plugin pack")
	}

	if name == "" {
		name = file.New(source).Base()
	}

	// The subpath scoping here is redundant (we already scoped above for validation)
	// but harmless — CopySource produces the same effectiveSource path.
	target, err := packs.CopySource(w, source, pluginsDir, name, subpath, "plugin")
	if err != nil {
		return err
	}

	// Post-copy: re-discover in copied target for vendoring.
	installed, err := config.DiscoverPlugins(folder.New(target))
	if err != nil {
		debug.Log("[plugins:add-local] discover: %v", err)
	}

	debug.Log("[plugins:add-local] discovered %d plugin(s)", len(installed))

	// Vendor dependencies if requested — each plugin dir independently.
	if vendor {
		for _, f := range installed {
			pluginDir := folder.FromFile(f).Path()

			debug.Log("[plugins:add-local] vendoring %s", pluginDir)
			fmt.Fprintf(w, "Vendoring %s...\n", pluginDir)

			if err := pluginspkg.Vendor(pluginDir); err != nil {
				folder.New(target).Remove()

				return fmt.Errorf("vendoring %s: %w", pluginDir, err)
			}
		}
	}

	// Verify SDK compatibility (runs even with --no-vendor: local dirs may have vendor/).
	for _, f := range installed {
		pluginDir := folder.FromFile(f).Path()

		if err := checkSDKCompat(pluginDir, name); err != nil {
			folder.New(target).Remove()

			return err
		}
	}

	debug.Log("[plugins:add-local] SDK compatibility OK")

	fmt.Fprintf(w, "Installed %d plugin(s) from %q (local)\n", len(installed), name)

	for _, f := range installed {
		pluginDir := folder.FromFile(f).Path()
		pluginName := folder.FromFile(f).Base()

		caps, probeErr := pluginspkg.ProbeCapabilities(pluginDir)
		if probeErr != nil {
			debug.Log("[plugin] probe %s: %v", pluginName, probeErr)

			fmt.Fprintf(w, "  %s (probe failed)\n", pluginName)
		} else {
			fmt.Fprintf(w, "  %s%s\n", pluginName, formatCapsSummary(caps))
		}
	}

	return nil
}

func update(flags *core.Flags) *cli.Command {
	return &cli.Command{
		Name:  "update",
		Usage: "Update git-sourced plugins",
		Description: heredoc.Doc(`
			Pull the latest changes for plugins installed from git repositories.

			Use --all to update all git-sourced plugins.
			Local plugins (installed from a path) cannot be updated.
		`),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "all",
				Usage:       "Update all git-sourced plugins",
				Destination: &flags.Plugins.Update.All,
			},
			&cli.BoolFlag{
				Name:        "no-vendor",
				Usage:       "Skip go mod tidy and go mod vendor after updating",
				Destination: &flags.Plugins.Update.NoVendor,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := core.GetFlags()

			cfg, _, err := config.New(flags.ConfigOptions(), config.PartPlugins)
			if err != nil {
				return err
			}

			vendor := !flags.Plugins.Update.NoVendor

			if flags.Plugins.Update.All {
				return updateAll(cmd.Writer, cfg.Plugins, vendor)
			}

			if cmd.Args().Len() == 0 {
				return errors.New("specify plugin name(s) or use --all")
			}

			var errs []error

			for _, name := range cmd.Args().Slice() {
				if err := updateOne(cmd.Writer, cfg.Plugins, name, vendor); err != nil {
					errs = append(errs, err)
				}
			}

			if len(errs) > 0 {
				return fmt.Errorf("updating %d plugin(s):\n%w", len(errs), errors.Join(errs...))
			}

			return nil
		},
	}
}

func updateOne(w io.Writer, pp config.StringCollection[config.Plugin], name string, vendor bool) error {
	p := pp.Get(name)

	// If not found by plugin name, try pack name.
	if p == nil {
		packPlugins := config.ByPack(pp, name)
		if len(packPlugins) == 0 {
			return fmt.Errorf(
				"plugin or pack %q not found\nAvailable plugins: %v\nAvailable packs: %v",
				name, pp.Names(), config.PackNames(pp),
			)
		}

		p = &packPlugins[0]
	}

	if !p.HasOrigin() {
		return fmt.Errorf("plugin %q has no origin — installed from a local path, cannot update", name)
	}

	// Determine display label — use pack terminology when multiple plugins share the origin.
	label := fmt.Sprintf("%q", p.Name)
	packPlugins := config.ByPack(pp, folder.New(p.OriginDir).Base())

	if len(packPlugins) > 1 {
		label = fmt.Sprintf("pack %q", folder.New(p.OriginDir).Base())
	}

	result, err := packs.PullUpdate(w, p.OriginDir, p.Origin, label)
	if err != nil {
		return err
	}

	if !result.Changed {
		return nil
	}

	// Discover plugins across all subpath roots (or entire clone if none).
	var found files.Files

	for _, root := range packs.DiscoveryRoots(p.OriginDir, p.Origin.Subpaths) {
		discovered, err := config.DiscoverPlugins(folder.New(root))
		if err != nil {
			debug.Log("[plugins:update] discover in %s: %v", root, err)
		}

		found = append(found, discovered...)
	}

	debug.Log("[plugins:update] discovered %d plugin(s)", len(found))

	// Vendor dependencies after pulling new code.
	if vendor {
		for _, f := range found {
			pluginDir := folder.FromFile(f).Path()

			debug.Log("[plugins:update] vendoring %s", pluginDir)
			fmt.Fprintf(w, "Vendoring %s...\n", pluginDir)

			if err := pluginspkg.Vendor(pluginDir); err != nil {
				return fmt.Errorf("vendoring %s: %w", pluginDir, err)
			}
		}
	}

	// Verify SDK compatibility after update.
	for _, f := range found {
		pluginDir := folder.FromFile(f).Path()

		if err := checkSDKCompat(pluginDir, folder.New(pluginDir).Base()); err != nil {
			return err
		}
	}

	debug.Log("[plugins:update] SDK compatibility OK")

	// Write origin AFTER all validation passes.
	origin := p.Origin

	origin.Commit = result.NewCommit

	if err := gitutil.WriteOrigin(p.OriginDir, origin); err != nil {
		fmt.Fprintf(w, "Warning: could not update origin: %v\n", err)
	}

	fmt.Fprintf(
		w,
		"Updated %s: %s → %s\n",
		label,
		gitutil.ShortCommit(result.OldCommit),
		gitutil.ShortCommit(result.NewCommit),
	)

	return nil
}

func updateAll(w io.Writer, pp config.StringCollection[config.Plugin], vendor bool) error {
	seen := map[string]bool{}

	var updated int

	for _, name := range pp.Names() {
		p := pp[name]
		if !p.HasOrigin() || seen[p.OriginDir] {
			continue
		}

		seen[p.OriginDir] = true

		if err := updateOne(w, pp, name, vendor); err != nil {
			fmt.Fprintf(w, "Error updating %s: %v\n", name, err)

			continue
		}

		updated++
	}

	if updated == 0 {
		fmt.Fprintln(w, "No git-sourced plugins to update.")
	}

	return nil
}

// checkSDKCompat verifies that a plugin's vendored SDK version matches the host.
// Returns nil if no vendored SDK exists (e.g., in-repo test plugins).
func checkSDKCompat(pluginDir, name string) error {
	caps, err := pluginspkg.ProbeCapabilities(pluginDir)
	if err != nil {
		return fmt.Errorf("plugin %q: probing: %w", name, err)
	}

	if caps.SDKVersion == "" {
		debug.Log("[plugins:sdk] %q: no vendored SDK (skipping check)", name)

		return nil
	}

	debug.Log("[plugins:sdk] %q: vendored=%s host=%s", name, caps.SDKVersion, version.Version)

	if err := pluginspkg.IsSDKCompatible(caps.SDKVersion, version.Version); err != nil {
		return fmt.Errorf("plugin %q: %w", name, err)
	}

	return nil
}

func remove() *cli.Command {
	return &cli.Command{
		Name:  "remove",
		Usage: "Remove installed plugins or packs",
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() < 1 {
				return errors.New("expected at least 1 argument")
			}

			flags := core.GetFlags()

			cfg, _, err := config.New(flags.ConfigOptions(), config.PartPlugins)
			if err != nil {
				return err
			}

			var errs []error

			for _, name := range cmd.Args().Slice() {
				if err := removeOne(cmd.Writer, cfg.Plugins, name); err != nil {
					errs = append(errs, err)
				}
			}

			if len(errs) > 0 {
				return fmt.Errorf("removing %d plugin(s):\n%w", len(errs), errors.Join(errs...))
			}

			return nil
		},
	}
}

func removeOne(w io.Writer, pp config.StringCollection[config.Plugin], name string) error {
	debug.Log("[plugins:remove] removing %q", name)

	// Try plugin lookup first.
	if p := pp.Get(name); p != nil {
		if p.IsPack() {
			// Pack member — refuse, direct user to remove the pack.
			packName := p.PackName()
			packPlugins := config.ByPack(pp, packName)

			var others []string

			for _, sibling := range packPlugins {
				if sibling.Name != name {
					others = append(others, sibling.Name)
				}
			}

			return fmt.Errorf(
				"%s belongs to pack %q (also contains: %s)\nUse: aura plugins remove %s",
				name, packName, strings.Join(others, ", "), packName,
			)
		}

		// Standalone plugin — remove its directory.
		if err := folder.New(p.Dir()).Remove(); err != nil {
			return fmt.Errorf("removing %q: %w", name, err)
		}

		fmt.Fprintf(w, "Removed plugin %q (%s)\n", name, p.Dir())

		return nil
	}

	// Try pack lookup.
	packPlugins := config.ByPack(pp, name)
	if len(packPlugins) == 0 {
		return fmt.Errorf("plugin or pack %q not found", name)
	}

	originDir := packPlugins[0].OriginDir

	if err := folder.New(originDir).Remove(); err != nil {
		return fmt.Errorf("removing pack %q: %w", name, err)
	}

	var pluginNames []string

	for _, p := range packPlugins {
		pluginNames = append(pluginNames, p.Name)
	}

	fmt.Fprintf(w, "Removed pack %q (%d plugins: %s)\n", name, len(packPlugins), strings.Join(pluginNames, ", "))

	return nil
}

// formatCapsSummary returns a parenthesized summary of plugin capabilities for install output.
// Returns empty string if the plugin has no discoverable capabilities.
func formatCapsSummary(caps pluginspkg.Capabilities) string {
	var parts []string

	if len(caps.Hooks) > 0 {
		parts = append(parts, "hooks: "+strings.Join(caps.Hooks, ", "))
	}

	if caps.ToolName != "" {
		parts = append(parts, "tool: "+caps.ToolName)
	}

	if caps.CommandName != "" {
		parts = append(parts, "command: /"+caps.CommandName)
	}

	if len(parts) == 0 {
		return ""
	}

	return " (" + strings.Join(parts, ", ") + ")"
}
