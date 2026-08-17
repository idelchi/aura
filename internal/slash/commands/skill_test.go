package commands

import (
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/godyl/pkg/path/file"
)

func TestSkillCommandForwardsToModel(t *testing.T) {
	t.Parallel()

	command := Skill()
	if command.Name != "/skill" || command.Hints != "<name>" || !command.Forward {
		t.Fatalf("unexpected command contract: %#v", command)
	}
}

func TestSkillPrompt(t *testing.T) {
	t.Parallel()

	var configured config.Skill
	configured.Metadata.Name = "greet"
	configured.Metadata.Description = "Greet someone"
	configured.Body = "Read {{ .Skill.Dir }}/references/greeting.md."
	configured.Dir = "/tmp/aura skills/greet"

	skills := config.Collection[config.Skill]{file.File("greet/SKILL.md"): configured}
	prompt, err := skillPrompt(skills, "GREET")
	if err != nil {
		t.Fatalf("invoking skill: %v", err)
	}

	if !strings.Contains(prompt, `explicitly invoked skill "greet"`) ||
		!strings.Contains(prompt, "/tmp/aura skills/greet/references/greeting.md") {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
}

func TestSkillPromptRejectsUnknownSkill(t *testing.T) {
	t.Parallel()

	_, err := skillPrompt(config.Collection[config.Skill]{}, "missing")
	if err == nil || !strings.Contains(err.Error(), `unknown skill "missing"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}
