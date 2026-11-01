package assistant

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/idelchi/aura/internal/injector"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/stream"
	"github.com/idelchi/aura/pkg/llm/tool/call"
	"github.com/idelchi/aura/pkg/llm/usage"
	"github.com/idelchi/aura/pkg/providers"
)

func TestCompactionReloadsCurrentAssignmentFiles(t *testing.T) {
	a := selectionAssistant(t)
	path := filepath.Join(t.TempDir(), "assignment.md")
	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("Old assignment and old revision")
	key, configured := a.cfg.Agents.GetWithKey(a.agent.Name)
	configured.Metadata.Files = []string{path}
	a.cfg.Agents[key] = *configured
	if err := a.RebuildState(); err != nil {
		t.Fatal(err)
	}
	a.cfg.Features.Compaction.Prompt = "Compaction"
	a.cfg.Features.Compaction.Chunks = 1
	a.resolved.compactModel = &model.Model{Name: "test"}
	a.builder.AddUserMessage(t.Context(), "Historical teaching from a different checkout", 12)
	a.builder.AddUserMessage(t.Context(), "Current task: cadi_loan_forgiven", 12)
	write("Current checkout /work/cadi; revision actual-123; scenario cadi_loan_forgiven; use native JSON arrays")
	a.agent.Provider = compactionProbe{Provider: a.agent.Provider, chat: func(context.Context) (message.Message, usage.Usage, error) {
		return message.Message{Content: "A deliberately incomplete summary."}, usage.Usage{}, nil
	}}
	if err := a.CompactWith(t.Context(), true, 0); err != nil {
		t.Fatal(err)
	}
	system := a.builder.History()[0].Content
	for _, fact := range []string{"Current checkout /work/cadi", "actual-123", "cadi_loan_forgiven", "native JSON arrays"} {
		if !strings.Contains(system, fact) {
			t.Fatalf("compaction lost the anchored fact %q", fact)
		}
	}
	if strings.Contains(system, "Old assignment and old revision") {
		t.Fatal("compaction retained the stale file contents")
	}
}

// compactionProbe replaces only Chat; configuration/model selection use Aura's
// real assistant and cached model resolution.
type compactionProbe struct {
	providers.Provider                                                             // other provider methods are unchanged
	chat               func(context.Context) (message.Message, usage.Usage, error) // deterministic response
}

// Chat returns the controlled response without contacting an inference service.
func (p compactionProbe) Chat(ctx context.Context, _ request.Request, _ stream.Func) (message.Message, usage.Usage, error) {
	return p.chat(ctx)
}

// TestCompactionFailureBudgets covers invalid output, time limits and the outer
// recovery loop: neither should multiply into nested transcript retries.
func TestCompactionFailureBudgets(t *testing.T) {
	for _, timed := range []bool{false, true} {
		t.Run(map[bool]string{false: "tool call", true: "deadline"}[timed], func(t *testing.T) {
			a := selectionAssistant(t)
			a.cfg.Features.Compaction.Prompt = "Compaction"
			a.cfg.Features.Compaction.Chunks = 1
			a.cfg.Features.Compaction.Timeout = 50 * time.Millisecond
			a.cfg.Features.Compaction.TruncationRetries = []int{150, 100, 50, 0}
			a.resolved.compactModel = &model.Model{Name: "test"}
			a.builder.AddUserMessage(t.Context(), "Assess these logs", 4)
			calls := 0
			a.agent.Provider = compactionProbe{Provider: a.agent.Provider, chat: func(ctx context.Context) (message.Message, usage.Usage, error) {
				calls++
				if timed {
					<-ctx.Done()
					return message.Message{}, usage.Usage{}, ctx.Err()
				}
				return message.Message{Calls: []call.Call{{ID: "invalid", Name: "Gotify"}}}, usage.Usage{}, nil
			}}
			err := a.RecoverCompaction(t.Context(), 0)
			if calls != 1 || err == nil {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
			if timed && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("lost deadline: %v", err)
			}
			if !timed && !errors.Is(err, ErrCompactionExhausted) {
				t.Fatalf("lost compaction failure: %v", err)
			}
		})
	}
}

// TestCompactionReceipts keeps real acknowledgements outside model-authored prose,
// as fixed excerpts that say how much of each response is not shown.
func TestCompactionReceipts(t *testing.T) {
	got := compactionReceipts([]injector.ToolCall{
		{Name: "Gotify", Result: "Notification suppressed [INFO]: task (minimum level: ERROR)"},
		{Name: "logs", Result: strings.Repeat("x", 500)},
		{Name: "Bash", Error: "exit status 1: " + strings.Repeat("é", 300)},
	})
	if !strings.Contains(got, "Notification suppressed") || strings.Contains(got, "Notification sent") {
		t.Fatal(got)
	}
	if !strings.Contains(got, "[receipt excerpt: first 200 of 500 bytes; the rest is not shown]:\n"+strings.Repeat("x", receiptExcerptBytes)) {
		t.Fatalf("long receipt not excerpted with its size: %s", got)
	}
	if !strings.Contains(got, "Execution failed: exit status 1") || !strings.Contains(got, "of 633 bytes") || !utf8.ValidString(got) {
		t.Fatalf("failed call not excerpted on a character boundary: %s", got)
	}
	if strings.Contains(got, "Notification suppressed [INFO]: task (minimum level: ERROR)\" [receipt") {
		t.Fatal("a complete receipt was marked as an excerpt")
	}
	if !strings.Contains(compactionReceipts(nil), "No tools were invoked") {
		t.Fatal("missing no-delivery evidence")
	}
}
