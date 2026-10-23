package assistant

import (
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/agent"
)

// Task workdirs must reach prompt composition and freshly constructed helper
// agents as well as execution, including when returning to the configured dir.
func TestWorkDirPromptLifecycle(t *testing.T) {
	a := selectionAssistant(t)
	key, ag := a.cfg.Agents.GetWithKey(a.agent.Name)
	ag.Metadata.System = ""
	a.cfg.Agents[key] = *ag
	a.agent.System = ""
	for _, override := range []string{t.TempDir(), t.TempDir(), ""} {
		if err := a.SetWorkDir(override); err != nil {
			t.Fatal(err)
		}
		want := override
		if want == "" {
			want = a.configOpts.WorkDir
		}
		if a.WorkDir() != want || a.resolved.template.WorkDir != want || a.paths.Work != want {
			t.Fatalf("working directory diverged: execute=%q render=%q paths=%q want=%q",
				a.WorkDir(), a.resolved.template.WorkDir, a.paths.Work, want)
		}
		if !strings.Contains(a.builder.SystemPrompt(), "Working directory: "+want) {
			t.Fatal("system prompt has wrong task directory")
		}
		helper, err := agent.New(a.cfg, a.paths, a.rt, a.agent.Name)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(helper.Prompt, "Working directory: "+want) {
			t.Fatal("helper prompt has wrong task directory")
		}
	}
}
