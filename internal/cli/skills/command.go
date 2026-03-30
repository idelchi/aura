package skills

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
	"github.com/idelchi/aura/pkg/frontmatter"
	"github.com/idelchi/aura/pkg/gitutil"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// Command creates the 'aura skills' command with add/update/remove subcommands.
func Command(flags *core.Flags) *cli.Command {
	return &cli.Command{
		Name:  "skills",
		Usage: "Manage skills",
		Description: heredoc.Doc(`
			Manage LLM-invocable skills.

			Skills are Markdown files with YAML frontmatter in .aura/skills/.
			The LLM invokes them via the Skill tool for multi-step instructions.

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
			addCmd(flags),
			updateCmd(flags),
			removeCmd(),
		},
	}
}

func addCmd(flags *core.Flags) *cli.Command {
	return &cli.Command{
		Name:  "add",
		Usage: "Install skills from git URLs, local paths, or .md files",
		Description: heredoc.Doc(`
			Install skills from git repositories, local directories, or single .md files.

			Git sources (HTTPS or SSH URLs) are cloned into the skills directory.
			Local directories are copied into the skills directory.
			Single .md files are wrapped in a directory and copied.

			The skill name defaults to the last segment of the source URL
			(stripped of "aura-skill-" prefix). Override with --name (single source only).

			Multiple sources can be provided when --name and --ref are not set.
		`),
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "name",
				Usage:       "Override the skill directory name",
				Destination: &flags.Skills.Add.Name,
			},
			&cli.BoolFlag{
				Name:        "global",
				Usage:       "Install to global skills directory (~/.aura)",
				Destination: &flags.Skills.Add.Global,
			},
			&cli.StringFlag{
				Name:        "ref",
				Usage:       "Git branch or tag to clone",
				Destination: &flags.Skills.Add.Ref,
			},
			&cli.StringSliceFlag{
				Name:        "subpath",
				Usage:       "Subdirectory within the source to use as skill root (repeatable)",
				Destination: &flags.Skills.Add.Subpath,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() < 1 {
				return errors.New("expected at least 1 argument")
			}

			flags := core.GetFlags()

			args := cmd.Args().Slice()

			if len(args) > 1 &&
				(flags.Skills.Add.Name != "" || flags.Skills.Add.Ref != "" || len(flags.Skills.Add.Subpath) > 0) {
				return errors.New("--name, --ref, and --subpath cannot be used with multiple sources")
			}

			if flags.Skills.Add.Name != "" && len(flags.Skills.Add.Subpath) > 1 {
				return errors.New("--name cannot be used with multiple --subpath values")
			}

			subpaths, err := packs.CleanSubpaths(flags.Skills.Add.Subpath)
			if err != nil {
				return err
			}

			skillsHome := flags.WriteHome()
			if flags.Skills.Add.Global {
				skillsHome = flags.Home
			}

			if skillsHome == "" {
				return errors.New("no config directory found (run 'aura init' first)")
			}

			skillsDir := folder.New(skillsHome, "skills").Path()

			var errs []error

			for _, source := range args {
				if gitutil.IsGitURL(source) {
					err := addGit(cmd.Writer, source, skillsDir, flags.Skills.Add.Name, flags.Skills.Add.Ref, subpaths)
					if err != nil {
						errs = append(errs, err)
					}
				} else {
					// Local paths: use first subpath only (local copies are scoped to a single path).
					var subpath string

					if len(subpaths) > 0 {
						subpath = subpaths[0]
					}

					err := addLocal(cmd.Writer, source, skillsDir, flags.Skills.Add.Name, subpath)
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

func addGit(w io.Writer, source, skillsDir, name, ref string, subpaths []string) error {
	if name == "" {
		name = packs.DeriveName(source, subpaths, "aura-skill-")
	}

	result, err := packs.CloneSource(w, source, skillsDir, name, ref, subpaths, "skill")
	if err != nil {
		return err
	}

	// Validate skills across all subpath roots (or entire clone if none).
	for _, root := range packs.DiscoveryRoots(result.TargetDir, subpaths) {
		if err := validateSkillDir(root); err != nil {
			folder.New(result.TargetDir).Remove()

			return err
		}
	}

	packs.WriteCloneOrigin(w, result.TargetDir, source, ref, result.Commit, subpaths)

	fmt.Fprintf(w, "Installed skill %q\n", name)

	if result.Commit != "" {
		fmt.Fprintf(w, "Commit: %s\n", gitutil.ShortCommit(result.Commit))
	}

	return nil
}

func addLocal(w io.Writer, source, skillsDir, name, subpath string) error {
	info, err := file.New(source).Info()
	if err != nil {
		return fmt.Errorf("source %q does not exist: %w", source, err)
	}

	if !info.IsDir() {
		if subpath != "" {
			return errors.New("--subpath cannot be used with file sources")
		}

		return addLocalFile(w, source, skillsDir, name)
	}

	return addLocalDir(w, source, skillsDir, name, subpath)
}

func addLocalDir(w io.Writer, source, skillsDir, name, subpath string) error {
	if name == "" {
		name = file.New(source).Base()
	}

	// Pre-copy: validate skill frontmatter on (scoped) source.
	effectiveSource := source

	if subpath != "" {
		effectiveSource = folder.New(source, subpath).Path()

		if !folder.New(effectiveSource).Exists() {
			return fmt.Errorf("subpath %q does not exist in %s", subpath, source)
		}
	}

	if err := validateSkillDir(effectiveSource); err != nil {
		return fmt.Errorf("source: %w", err)
	}

	// The subpath scoping here is redundant (we scoped above for validation)
	// but harmless — CopySource produces the same effectiveSource path.
	if _, err := packs.CopySource(w, source, skillsDir, name, subpath, "skill"); err != nil {
		return err
	}

	debug.Log("[skills:add-local] installed %q", name)
	fmt.Fprintf(w, "Installed skill %q (local)\n", name)

	return nil
}

func addLocalFile(w io.Writer, source, skillsDir, name string) error {
	if file.New(source).Extension() != "md" {
		return fmt.Errorf("source %q is not a .md file", source)
	}

	if err := validateSkillFile(source); err != nil {
		return err
	}

	if name == "" {
		name = strings.TrimSuffix(file.New(source).Base(), ".md")
	}

	targetDir := folder.New(skillsDir, name)
	target := targetDir.Path()

	if targetDir.Exists() {
		return fmt.Errorf("skill %q already exists at %s", name, target)
	}

	if err := targetDir.Create(); err != nil {
		return fmt.Errorf("creating %s: %w", target, err)
	}

	data, err := file.New(source).Read()
	if err != nil {
		return fmt.Errorf("reading %s: %w", source, err)
	}

	targetFile := targetDir.WithFile(file.New(source).Base()).Path()
	if err := file.New(targetFile).Write(data, 0o644); err != nil {
		targetDir.Remove()

		return fmt.Errorf("writing %s: %w", targetFile, err)
	}

	fmt.Fprintf(w, "Installed skill %q (local)\n", name)

	return nil
}

func updateCmd(flags *core.Flags) *cli.Command {
	return &cli.Command{
		Name:  "update",
		Usage: "Update git-sourced skills",
		Description: heredoc.Doc(`
			Pull the latest changes for skills installed from git repositories.

			Use --all to update all git-sourced skills.
			Local skills (installed from a path) cannot be updated.
		`),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "all",
				Usage:       "Update all git-sourced skills",
				Destination: &flags.Skills.Update.All,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := core.GetFlags()

			cfg, _, err := config.New(flags.ConfigOptions(), config.PartSkills)
			if err != nil {
				return err
			}

			if flags.Skills.Update.All {
				return updateAll(cmd.Writer, cfg.Skills)
			}

			if cmd.Args().Len() == 0 {
				return errors.New("specify skill name(s) or use --all")
			}

			var errs []error

			for _, name := range cmd.Args().Slice() {
				if err := updateOne(cmd.Writer, cfg.Skills, name); err != nil {
					errs = append(errs, err)
				}
			}

			if len(errs) > 0 {
				return fmt.Errorf("updating %d skill(s):\n%w", len(errs), errors.Join(errs...))
			}

			return nil
		},
	}
}

func updateOne(w io.Writer, ss config.Collection[config.Skill], name string) error {
	f, skill := ss.GetWithKey(name)
	if skill == nil {
		return fmt.Errorf("skill %q not found\nAvailable: %v", name, ss.Names())
	}

	dir := f.Dir()

	// Walk up to find origin (subpath skills have origin at clone root, not skill dir).
	origin, originDir, found := gitutil.FindOrigin(dir)
	if !found {
		return fmt.Errorf("skill %q has no origin — installed from a local path, cannot update", name)
	}

	result, err := packs.PullUpdate(w, originDir, origin, fmt.Sprintf("%q", name))
	if err != nil {
		return err
	}

	if !result.Changed {
		return nil
	}

	// Skills has no post-pull validation → write origin immediately.
	origin.Commit = result.NewCommit

	if err := gitutil.WriteOrigin(originDir, origin); err != nil {
		fmt.Fprintf(w, "Warning: could not update origin: %v\n", err)
	}

	fmt.Fprintf(
		w,
		"Updated %s: %s → %s\n",
		name,
		gitutil.ShortCommit(result.OldCommit),
		gitutil.ShortCommit(result.NewCommit),
	)

	return nil
}

func updateAll(w io.Writer, ss config.Collection[config.Skill]) error {
	seen := map[string]bool{}

	var updated int

	for _, name := range ss.Names() {
		f, _ := ss.GetWithKey(name)
		dir := f.Dir()

		// Walk up to find origin (dedup by clone root, not skill dir).
		_, originDir, found := gitutil.FindOrigin(dir)
		if !found {
			continue
		}

		if seen[originDir] {
			continue
		}

		seen[originDir] = true

		if err := updateOne(w, ss, name); err != nil {
			fmt.Fprintf(w, "Error updating %s: %v\n", name, err)

			continue
		}

		updated++
	}

	if updated == 0 {
		fmt.Fprintln(w, "No git-sourced skills to update.")
	}

	return nil
}

func removeCmd() *cli.Command {
	return &cli.Command{
		Name:  "remove",
		Usage: "Remove installed skills",
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() < 1 {
				return errors.New("expected at least 1 argument")
			}

			flags := core.GetFlags()

			cfg, _, err := config.New(flags.ConfigOptions(), config.PartSkills)
			if err != nil {
				return err
			}

			var errs []error

			for _, name := range cmd.Args().Slice() {
				if err := removeOne(cmd.Writer, cfg.Skills, name); err != nil {
					errs = append(errs, err)
				}
			}

			if len(errs) > 0 {
				return fmt.Errorf("removing %d skill(s):\n%w", len(errs), errors.Join(errs...))
			}

			return nil
		},
	}
}

func removeOne(w io.Writer, ss config.Collection[config.Skill], name string) error {
	debug.Log("[skills:remove] removing %q", name)

	f, skill := ss.GetWithKey(name)
	if skill == nil {
		return fmt.Errorf("skill %q not found\nAvailable: %v", name, ss.Names())
	}

	dir := f.Dir()

	// Check if this is a subpath skill with a clone root above the skill dir.
	if origin, originDir, found := gitutil.FindOrigin(dir); found && len(origin.Subpaths) > 0 {
		if err := folder.New(originDir).Remove(); err != nil {
			return fmt.Errorf("removing %s: %w", originDir, err)
		}

		fmt.Fprintf(w, "Removed skill %q (%s)\n", name, originDir)

		return nil
	}

	// If the skill file is directly in the skills/ config dir, remove just the file.
	// Otherwise it's in a subdirectory (from `skills add`) — remove the whole directory.
	if folder.New(dir).Base() == "skills" {
		if err := f.Remove(); err != nil {
			return fmt.Errorf("removing %s: %w", f.Path(), err)
		}

		fmt.Fprintf(w, "Removed skill %q (%s)\n", name, f.Path())
	} else {
		if err := folder.New(dir).Remove(); err != nil {
			return fmt.Errorf("removing %s: %w", dir, err)
		}

		fmt.Fprintf(w, "Removed skill %q (%s)\n", name, dir)
	}

	return nil
}

// validateSkillDir checks that a directory contains at least one valid skill .md file.
func validateSkillDir(dir string) error {
	entries, err := folder.New(dir).ListFiles()
	if err != nil {
		return fmt.Errorf("reading directory %s: %w", dir, err)
	}

	for _, e := range entries {
		if e.Extension() != "md" {
			continue
		}

		if err := validateSkillFile(e.Path()); err == nil {
			return nil
		}
	}

	return errors.New("no valid skill files found (need .md with name + description frontmatter)")
}

// validateSkillFile checks that a .md file has valid skill frontmatter.
func validateSkillFile(path string) error {
	var meta struct {
		Name        string `validate:"required"`
		Description string `validate:"required"`
	}

	_, err := frontmatter.Load(file.New(path), &meta)
	if err != nil {
		return fmt.Errorf("invalid frontmatter in %s: %w", path, err)
	}

	if meta.Name == "" {
		return fmt.Errorf("missing required field 'name' in %s", path)
	}

	if meta.Description == "" {
		return fmt.Errorf("missing required field 'description' in %s", path)
	}

	return nil
}
