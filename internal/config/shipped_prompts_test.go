package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/prompts"
	"github.com/idelchi/aura/pkg/frontmatter"
	"github.com/idelchi/godyl/pkg/path/file"
)

// Render the shipped templates with representative callable tool sets, including
// a deferred TodoCreate that must not receive callable-tool instructions yet.
func TestShippedPromptToolGuidance(t *testing.T) {
	for _, system := range []string{"agentic", "lite", "chat"} {
		for _, mode := range []string{"edit", "plan"} {
			for _, tools := range []struct {
				name     string
				eager    []string
				deferred string
			}{
				{name: "none"},
				{name: "files", eager: []string{"Read", "Write", "Edit", "Bash"}},
				{name: "patch", eager: []string{"Read", "Patch"}},
				{name: "todos", eager: []string{"TodoCreate", "TodoList", "TodoProgress"}},
				{name: "deferred", deferred: "TodoCreate is available through LoadTools."},
			} {
				t.Run(system+"/"+mode+"/"+tools.name, func(t *testing.T) {
					ts := prompts.NewTemplateSet("system")
					for name, path := range map[string]string{
						"system": "prompts/system/" + system + ".md",
						"mode":   "modes/" + mode + ".md",
					} {
						_, body, err := frontmatter.LoadRaw(file.New(filepath.Join("../../samples/config", path)))
						if err != nil {
							t.Fatal(err)
						}
						ts.Register(name, body)
					}
					ts.Register("agent", "")
					text, err := ts.Render(TemplateData{
						WorkDir: "/tmp/task workspace", Mode: ModeData{Name: mode},
						Tools:      NewToolsData(tools.eager, tools.deferred),
						ReadBefore: ReadBeforeData{Write: true, Delete: true},
					})
					if err != nil {
						t.Fatal(err)
					}
					if strings.Count(text, "Working directory: /tmp/task workspace") != 1 {
						t.Fatal("working directory missing or duplicated")
					}
					checks := map[string]bool{
						"## File Editing Policy":                    tools.name == "files" || tools.name == "patch",
						"Read existing files before deleting them.": tools.name == "patch",
						"You MUST execute the TodoCreate tool":      mode == "plan" && tools.name == "todos",
						"you MUST periodically review it":           mode == "edit" && tools.name == "todos",
						"Each Patch or Write clears":                false,
					}
					for phrase, want := range checks {
						if strings.Contains(text, phrase) != want {
							t.Errorf("guidance %q present=%v, want %v", phrase, !want, want)
						}
					}
				})
			}
		}
	}
}
