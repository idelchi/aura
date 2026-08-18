package config_test

import (
	"testing"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/godyl/pkg/path/file"
	"go.yaml.in/yaml/v4"
)

func TestSkillInvocationPolicy(t *testing.T) {
	t.Parallel()

	var automatic config.Skill
	automatic.Metadata.Name = "automatic"

	var explicit config.Skill
	explicit.Metadata.Name = "explicit"
	explicit.Metadata.Explicit = true

	if !automatic.IsModelInvocable() || automatic.Invocation() != "model + /skill" {
		t.Fatalf("unexpected default policy: model=%t display=%q", automatic.IsModelInvocable(), automatic.Invocation())
	}

	if explicit.IsModelInvocable() || explicit.Invocation() != "/skill only" {
		t.Fatalf("unexpected explicit policy: model=%t display=%q", explicit.IsModelInvocable(), explicit.Invocation())
	}

	skills := config.Collection[config.Skill]{
		file.File("automatic/SKILL.md"): automatic,
		file.File("explicit/SKILL.md"):  explicit,
	}
	modelSkills := skills.Filter(config.Skill.IsModelInvocable)

	if len(modelSkills) != 1 || modelSkills.Get("automatic") == nil || modelSkills.Get("explicit") != nil {
		t.Fatalf("unexpected model-facing skills: %v", modelSkills.Names())
	}
}

func TestSkillExplicitFrontmatter(t *testing.T) {
	t.Parallel()

	var skill config.Skill
	if err := yaml.Unmarshal([]byte("name: user-only\ndescription: Run only when requested\nexplicit: true\n"), &skill.Metadata); err != nil {
		t.Fatalf("unmarshal skill metadata: %v", err)
	}

	if !skill.Metadata.Explicit || skill.IsModelInvocable() {
		t.Fatalf("explicit policy was not loaded: %+v", skill.Metadata)
	}
}
