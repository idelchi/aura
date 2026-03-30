package config

import (
	"fmt"
	"strings"

	"github.com/idelchi/aura/pkg/frontmatter"
	"github.com/idelchi/aura/pkg/gitutil"
	"github.com/idelchi/godyl/pkg/path/files"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// Skill represents a user-defined LLM-invocable capability loaded from a Markdown file.
// Skills are invoked by the LLM via the Skill meta-tool. The description is visible
// in the tool schema for progressive disclosure; the full body is returned on invocation.
type Skill struct {
	Metadata struct {
		Name        string `validate:"required"`
		Description string `validate:"required"`
	}
	Body string
}

// Name returns the skill's identifier.
func (s Skill) Name() string { return s.Metadata.Name }

// Display returns a one-line summary for listing.
func (s Skill) Display() string {
	return fmt.Sprintf("%-20s  %s", s.Metadata.Name, s.Metadata.Description)
}

// loadSkills parses skill Markdown files and returns a Collection keyed by source file.
func loadSkills(ff files.Files) (Collection[Skill], error) {
	result := make(Collection[Skill])

	for _, file := range ff {
		var skill Skill

		body, err := frontmatter.Load(file, &skill.Metadata)
		if err != nil {
			return nil, fmt.Errorf("skill %s: %w", file, err)
		}

		skill.Body = strings.TrimSpace(body)

		// Filter by origin subpath (git-sourced skills with --subpath).
		skillDir := folder.New(file.Dir())
		if origin, originDir, found := gitutil.FindOrigin(skillDir.Path()); found && len(origin.Subpaths) > 0 {
			inScope := false

			for _, sp := range origin.Subpaths {
				scopedRoot := folder.New(originDir, sp)

				rel, err := skillDir.RelativeTo(scopedRoot)
				if err == nil && !strings.HasPrefix(rel.Path(), "..") {
					inScope = true

					break
				}
			}

			if !inScope {
				continue
			}
		}

		for k, existing := range result {
			if strings.EqualFold(existing.Metadata.Name, skill.Metadata.Name) {
				delete(result, k)

				break
			}
		}

		result[file] = skill
	}

	return result, nil
}
