package skill_test

import (
	"context"
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/tools/skill"
	"github.com/idelchi/godyl/pkg/path/file"
)

func makeSkills() config.Collection[config.Skill] {
	var greet config.Skill
	greet.Metadata.Name = "greet"
	greet.Metadata.Description = "Say hello"
	greet.Body = "Hello, world!"

	var commit config.Skill
	commit.Metadata.Name = "commit"
	commit.Metadata.Description = "Make a commit"
	commit.Body = "git add && git commit"

	var explicit config.Skill
	explicit.Metadata.Name = "dangerous"
	explicit.Metadata.Description = "Perform a user-directed operation"
	explicit.Metadata.Explicit = true
	explicit.Body = "Do exactly what the user requested."

	return config.Collection[config.Skill]{
		file.File("greet.md"):     greet,
		file.File("commit.md"):    commit,
		file.File("dangerous.md"): explicit,
	}
}

func TestExecuteFound(t *testing.T) {
	t.Parallel()

	tool := skill.New(makeSkills())

	got, err := tool.Execute(context.Background(), map[string]any{"name": "greet"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if got != "Hello, world!" {
		t.Errorf("result = %q, want %q", got, "Hello, world!")
	}
}

func TestExecuteNotFound(t *testing.T) {
	t.Parallel()

	tool := skill.New(makeSkills())

	_, err := tool.Execute(context.Background(), map[string]any{"name": "unknown"})
	if err == nil {
		t.Fatal("expected error for unknown skill, got nil")
	}

	if !strings.Contains(err.Error(), "unknown skill") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "unknown skill")
	}
}

func TestExecuteCaseInsensitive(t *testing.T) {
	t.Parallel()

	tool := skill.New(makeSkills())

	got, err := tool.Execute(context.Background(), map[string]any{"name": "GREET"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if got != "Hello, world!" {
		t.Errorf("result = %q, want %q", got, "Hello, world!")
	}
}

func TestExecuteResolvesSkillDirectoryOnly(t *testing.T) {
	t.Parallel()

	skills := makeSkills()
	greet := skills[file.File("greet.md")]
	greet.Dir = "/tmp/aura skills/greet"
	greet.Body = "Read {{ .Skill.Dir }}/references/details.md; keep {{ .Other.Value }} literal."
	skills[file.File("greet.md")] = greet

	tool := skill.New(skills)

	got, err := tool.Execute(context.Background(), map[string]any{"name": "greet"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	want := "Read /tmp/aura skills/greet/references/details.md; keep {{ .Other.Value }} literal."
	if got != want {
		t.Errorf("result = %q, want %q", got, want)
	}
}

func TestSchemaIncludesSkillNames(t *testing.T) {
	t.Parallel()

	tool := skill.New(makeSkills())

	desc := tool.Schema().Description

	for _, name := range []string{"greet", "commit"} {
		if !strings.Contains(desc, name) {
			t.Errorf("Schema description missing skill %q, got: %s", name, desc)
		}
	}

	if strings.Contains(desc, "dangerous") {
		t.Errorf("schema exposes explicit-only skill, got: %s", desc)
	}
}

func TestExecuteRejectsExplicitOnlySkill(t *testing.T) {
	t.Parallel()

	tool := skill.New(makeSkills())

	_, err := tool.Execute(context.Background(), map[string]any{"name": "dangerous"})
	if err == nil || !strings.Contains(err.Error(), "unknown skill") {
		t.Fatalf("unexpected error: %v", err)
	}
}
