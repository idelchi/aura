package assistant

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/injector"
	"github.com/idelchi/aura/pkg/llm/generation"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/tool/call"
	"github.com/idelchi/aura/pkg/llm/usage"
	"github.com/idelchi/aura/pkg/providers"
)

// evidence returns n bytes of varied text, so local token estimates scale with size.
func evidence(n int) string {
	const unit = "stance 2.5 1.2 attack Heavy swing guard 10 "

	return strings.Repeat(unit, n/len(unit)+1)[:n]
}

// evidenceAssistant self-compacts on a 32K window with the given chunk count and
// tool-result lengths; every other compaction control is explicit here.
func evidenceAssistant(t *testing.T, chunks int, lengths ...int) *Assistant {
	t.Helper()

	a := selectionAssistant(t)
	c := &a.cfg.Features.Compaction
	c.Prompt, c.Agent, c.Chunks = "Compaction", "", chunks
	c.Threshold, c.MaxTokens = 80, 0
	c.ToolResultMaxLen, c.TruncationRetries = lengths[0], lengths[1:]
	c.Prune.Mode = config.PruneModeOff
	a.resolved.compactModel = &model.Model{Name: "test"}
	a.agent.Model.Context = 32768

	return a
}

// probe records every summarization request and answers with respond.
func probe(a *Assistant, respond func(n int, req request.Request) (message.Message, error)) *[]request.Request {
	return probeUsage(a, func(n int, req request.Request) (message.Message, usage.Usage, error) {
		msg, err := respond(n, req)

		return msg, usage.Usage{}, err
	})
}

// probeUsage records every summarization request and answers with respond, including usage.
func probeUsage(a *Assistant, respond func(n int, req request.Request) (message.Message, usage.Usage, error)) *[]request.Request {
	var seen []request.Request

	a.agent.Provider = compactionRequestProbe{Provider: a.agent.Provider, chat: func(req request.Request) (message.Message, usage.Usage, error) {
		seen = append(seen, req)

		return respond(len(seen), req)
	}}

	return &seen
}

// summarizeOK answers every request with a fixed summary.
func summarizeOK(int, request.Request) (message.Message, error) {
	return message.Message{Content: "Summary of earlier work."}, nil
}

// toolContent returns the single tool result carried by a request.
func toolContent(t *testing.T, req request.Request) string {
	t.Helper()

	for _, msg := range req.Messages {
		if msg.Role == roles.Tool {
			return msg.Content
		}
	}

	t.Fatal("request carries no tool result")

	return ""
}

// readHistory adds an inspected tool result followed by a recent answer that the
// preserved tail (keepLast 1) retains.
func readHistory(t *testing.T, a *Assistant, result string) {
	t.Helper()

	a.builder.AddUserMessage(t.Context(), "Inspect the encounter.", 5)
	a.builder.AddAssistantMessage(message.Message{Calls: []call.Call{{ID: "read", Name: "Read"}}})
	a.builder.AddToolResult(t.Context(), "Read", "read", result, 0)
	a.builder.AddAssistantMessage(message.Message{Content: "Latest answer."})
}

// TestCompactionEvidenceSelection sends full evidence when it fits and otherwise the
// largest configured length whose estimate fits, skipping larger ones without a call.
func TestCompactionEvidenceSelection(t *testing.T) {
	t.Run("full evidence when it fits", func(t *testing.T) {
		a := evidenceAssistant(t, 1, 65536, 4096)
		a.cfg.Features.Compaction.MaxTokens = 100000
		original := evidence(40000)
		readHistory(t, a, original)
		seen := probe(a, summarizeOK)

		if err := a.CompactWith(t.Context(), true, 1); err != nil {
			t.Fatal(err)
		}

		if len(*seen) != 1 || toolContent(t, (*seen)[0]) != original {
			t.Fatalf("calls=%d; evidence was not sent whole", len(*seen))
		}
	})

	t.Run("largest fitting length", func(t *testing.T) {
		lengths := []int{65536, 16384, 4096, 0}
		a := evidenceAssistant(t, 1, lengths...)
		a.cfg.Features.Compaction.MaxTokens = 8000
		readHistory(t, a, evidence(120000))
		toCompact, _ := splitHistory(a.builder.History(), 1)
		resolved, err := a.ResolveCompaction(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		seen := probe(a, summarizeOK)

		if err := a.CompactWith(t.Context(), true, 1); err != nil {
			t.Fatal(err)
		}

		chosen := -1
		for i, length := range lengths {
			if a.estimateRequest(t.Context(), buildCompactionMessages(resolved.prompt, toCompact, length, a.todoState())) <= 8000 {
				chosen = i

				break
			}
		}

		if chosen < 1 {
			t.Fatalf("fixture must need a smaller length; first fitting index %d", chosen)
		}

		want := fmt.Sprintf("[tool result excerpt: first %d of 120000 bytes shown", lengths[chosen])
		if len(*seen) != 1 || !strings.Contains(toolContent(t, (*seen)[0]), want) {
			t.Fatalf("calls=%d, want one request containing %q", len(*seen), want)
		}
	})

	t.Run("unknown budget sends the first length", func(t *testing.T) {
		a := evidenceAssistant(t, 1, 65536, 0)
		a.agent.Model.Context = 0
		readHistory(t, a, evidence(120000))
		seen := probe(a, summarizeOK)

		if err := a.CompactWith(t.Context(), true, 1); err != nil {
			t.Fatal(err)
		}

		if len(*seen) != 1 || !strings.Contains(toolContent(t, (*seen)[0]), "first 65536 of 120000 bytes") {
			t.Fatalf("calls=%d", len(*seen))
		}
	})
}

// TestCompactionProviderFailures steps down after a provider overflow and returns any
// other failure at once, leaving the history untouched.
func TestCompactionProviderFailures(t *testing.T) {
	t.Run("overflow steps down", func(t *testing.T) {
		a := evidenceAssistant(t, 1, 65536, 4096)
		a.cfg.Features.Compaction.MaxTokens = 100000
		readHistory(t, a, evidence(40000))
		seen := probe(a, func(n int, _ request.Request) (message.Message, error) {
			if n == 1 {
				return message.Message{}, providers.ErrContextExhausted
			}

			return message.Message{Content: "Summary."}, nil
		})

		if err := a.CompactWith(t.Context(), true, 1); err != nil {
			t.Fatal(err)
		}

		if len(*seen) != 2 || len(toolContent(t, (*seen)[0])) != 40000 ||
			!strings.Contains(toolContent(t, (*seen)[1]), "first 4096 of 40000 bytes") {
			t.Fatalf("calls=%d; overflow did not move to the next length", len(*seen))
		}
	})

	t.Run("other failures propagate", func(t *testing.T) {
		a := evidenceAssistant(t, 1, 65536, 4096)
		readHistory(t, a, evidence(4000))
		before := slices.Clone(a.builder.History())
		cause := errors.New("summary service unavailable")
		seen := probe(a, func(int, request.Request) (message.Message, error) { return message.Message{}, cause })

		err := a.CompactWith(t.Context(), true, 1)
		if !errors.Is(err, cause) || errors.Is(err, providers.ErrContextExhausted) || len(*seen) != 1 {
			t.Fatalf("calls=%d err=%v", len(*seen), err)
		}

		if !reflect.DeepEqual(before, a.builder.History()) {
			t.Fatal("a failed compaction changed the history")
		}

		*seen = nil
		if err := a.RecoverCompaction(t.Context(), 1); !errors.Is(err, ErrCompactionExhausted) || !errors.Is(err, cause) || len(*seen) != 1 {
			t.Fatalf("recovery calls=%d err=%v", len(*seen), err)
		}
	})
}

// TestCompactionRefusesOversizedText: user and assistant text is never shortened, so
// a request that cannot fit even without tool results fails clearly before any call
// (or before the chunk that cannot fit), leaves history unchanged and stops recovery.
func TestCompactionRefusesOversizedText(t *testing.T) {
	for _, chunks := range []int{1, 2} {
		t.Run(fmt.Sprintf("chunks=%d", chunks), func(t *testing.T) {
			a := evidenceAssistant(t, chunks, 65536, 0)
			a.builder.AddUserMessage(t.Context(), "First question.", 3)
			a.builder.AddAssistantMessage(message.Message{Content: "First answer."})
			a.builder.AddUserMessage(t.Context(), evidence(200000), 0)
			a.builder.AddAssistantMessage(message.Message{Content: "Latest answer."})
			before := slices.Clone(a.builder.History())
			seen := probe(a, summarizeOK)

			err := a.CompactWith(t.Context(), true, 1)
			if !errors.Is(err, ErrCompactionInputBudget) || errors.Is(err, providers.ErrContextExhausted) {
				t.Fatalf("err=%v", err)
			}

			// With chunks, the small first chunk is summarized before the second is refused.
			if len(*seen) != chunks-1 || !reflect.DeepEqual(before, a.builder.History()) {
				t.Fatalf("calls=%d; history changed=%t", len(*seen), !reflect.DeepEqual(before, a.builder.History()))
			}

			*seen = nil
			if err := a.RecoverCompaction(t.Context(), 1); !errors.Is(err, ErrCompactionExhausted) || !errors.Is(err, ErrCompactionInputBudget) || len(*seen) != chunks-1 {
				t.Fatalf("recovery calls=%d err=%v", len(*seen), err)
			}
		})
	}
}

// TestCompactionOutputAllowance keeps a configured output allowance free; one that
// fills the window is a configuration error that aborts recovery without a call.
func TestCompactionOutputAllowance(t *testing.T) {
	a := evidenceAssistant(t, 1, 200)
	readHistory(t, a, "short result")
	full := 32768
	a.agent.Model.Generation = &generation.Generation{MaxOutputTokens: &full}
	seen := probe(a, summarizeOK)

	if err := a.RecoverCompaction(t.Context(), 1); !errors.Is(err, ErrCompactionConfig) || len(*seen) != 0 {
		t.Fatalf("calls=%d err=%v", len(*seen), err)
	}
}

// TestCompactionChunkRequests: each chunk fits its own request; both prompt paths carry
// the same source-grounded rules, the previous summary and unshortened user text.
func TestCompactionChunkRequests(t *testing.T) {
	a := evidenceAssistant(t, 2, 65536, 4096)
	a.cfg.Features.Compaction.MaxTokens = 8000
	question := evidence(6000)
	a.builder.AddUserMessage(t.Context(), question, 0)
	a.builder.AddAssistantMessage(message.Message{Content: "First answer."})
	readHistory(t, a, evidence(120000))
	seen := probe(a, func(n int, _ request.Request) (message.Message, error) {
		return message.Message{Content: fmt.Sprintf("Summary %d.", n)}, nil
	})

	if err := a.CompactWith(t.Context(), true, 1); err != nil {
		t.Fatal(err)
	}

	if len(*seen) != 2 {
		t.Fatalf("calls=%d, want one per chunk", len(*seen))
	}

	first, second := (*seen)[0].Messages, (*seen)[1].Messages
	if first[1].Content != question {
		t.Fatal("user text was shortened")
	}

	if !strings.Contains(toolContent(t, (*seen)[1]), "first 4096 of 120000 bytes") {
		t.Fatal("the second chunk did not use the length that fits")
	}

	for i, msgs := range []message.Messages{first, second} {
		instruction := msgs[len(msgs)-1]
		if instruction.Role != roles.User || !strings.Contains(instruction.Content, compactionRules()) ||
			strings.Contains(instruction.Content, "EXACTLY") || strings.Contains(instruction.Content, "reproduce verbatim") {
			t.Fatalf("request %d instruction: %q", i+1, instruction.Content)
		}
	}

	if !strings.Contains(second[len(second)-1].Content, "Summary 1.") {
		t.Fatal("the second chunk lost the previous summary")
	}

	single := buildCompactionMessages("system", message.Messages{{Role: roles.User, Content: "Q"}}, 200, "")
	if !strings.Contains(single[len(single)-1].Content, compactionRules()) {
		t.Fatal("single-pass request lost the rules")
	}
}

// TestCompactionLifecycle replaces only the compacted prefix: retained messages stay
// byte-identical, including a large retained tool result, and receipts keep their
// fixed excerpt even when the summarizer reads full evidence.
func TestCompactionLifecycle(t *testing.T) {
	a := evidenceAssistant(t, 1, 65536, 0)
	a.builder.AddUserMessage(t.Context(), "Old question.", 3)
	a.builder.AddAssistantMessage(message.Message{Content: "Old answer."})
	retained := evidence(30000)
	readHistory(t, a, retained)
	a.loop.toolHistory = []injector.ToolCall{{Name: "Read", Result: evidence(5000)}}
	_, preserved := splitHistory(a.builder.History(), 4)
	seen := probe(a, summarizeOK)

	if err := a.CompactWith(t.Context(), true, 4); err != nil {
		t.Fatal(err)
	}

	history := a.builder.History().WithoutInternalMessages()
	if len(*seen) != 1 || history[0].Role != roles.System || len(history) != 2+len(preserved) {
		t.Fatalf("calls=%d rebuilt=%d", len(*seen), len(history))
	}

	summary := history[1].Content
	if !strings.Contains(summary, "Summary of earlier work.") ||
		!strings.Contains(summary, "[receipt excerpt: first 200 of 5000 bytes; the rest is not shown]") ||
		strings.Contains(summary, evidence(201)) {
		t.Fatalf("summary/receipts: %q", summary)
	}

	for i, msg := range preserved {
		got := history[2+i]
		if got.Role != msg.Role || got.Content != msg.Content || !reflect.DeepEqual(got.Calls, msg.Calls) {
			t.Fatalf("retained message %d changed", i)
		}
	}

	if history[len(history)-2].Content != retained {
		t.Fatal("retained tool result was not kept whole")
	}
}

func TestCompactionRefusesAbnormalSummaries(t *testing.T) {
	for _, finish := range []message.Finish{message.FinishError, message.FinishContentFilter, message.FinishToolCalls} {
		t.Run(finish.String(), func(t *testing.T) {
			a := evidenceAssistant(t, 1, 65536, 4096)
			readHistory(t, a, "Useful source evidence")
			before := slices.Clone(a.builder.History())
			seen := probe(a, func(int, request.Request) (message.Message, error) {
				return message.Message{Content: "Plausible but incomplete summary", Finish: finish}, nil
			})
			if err := a.CompactWith(t.Context(), true, 1); err == nil || !strings.Contains(err.Error(), finish.String()) {
				t.Fatalf("expected reported %s to reject the summary, got %v", finish, err)
			}
			if len(*seen) != 1 || !reflect.DeepEqual(before, a.builder.History()) {
				t.Fatal("abnormal summary retried or changed history")
			}
			*seen = nil
			if err := a.RecoverCompaction(t.Context(), 1); !errors.Is(err, ErrCompactionExhausted) || len(*seen) != 1 ||
				!reflect.DeepEqual(before, a.builder.History()) {
				t.Fatalf("recovery accepted an abnormal summary or shrank its preserved tail: %v", err)
			}
		})
	}
}

// TestCompactionRefusesCutOffSummaries: a summary the provider reports as stopped at a
// length limit never replaces the history. With room to gain it steps down; once a
// configured output allowance is used up, or the lengths run out, it fails and recovery
// stops. An unreported reason is accepted but not treated as verified.
func TestCompactionRefusesCutOffSummaries(t *testing.T) {
	cutOff := message.Message{Content: "Partial summ", Finish: message.FinishLength}

	t.Run("steps down for more room", func(t *testing.T) {
		a := evidenceAssistant(t, 1, 65536, 4096)
		a.cfg.Features.Compaction.MaxTokens = 100000
		readHistory(t, a, evidence(40000))
		seen := probe(a, func(n int, _ request.Request) (message.Message, error) {
			if n == 1 {
				return cutOff, nil
			}

			return message.Message{Content: "Complete summary.", Finish: message.FinishStop}, nil
		})

		if err := a.CompactWith(t.Context(), true, 1); err != nil {
			t.Fatal(err)
		}

		if len(*seen) != 2 || !strings.Contains(toolContent(t, (*seen)[1]), "first 4096 of 40000 bytes") ||
			!strings.Contains(a.builder.History()[1].Content, "Complete summary.") || strings.Contains(a.builder.History()[1].Content, "Partial summ") {
			t.Fatalf("calls=%d summary=%q", len(*seen), a.builder.History()[1].Content)
		}
	})

	for _, tc := range []struct {
		name  string
		limit *int
		calls int
	}{
		{name: "used-up output allowance fails at once", limit: new(1000), calls: 1},
		{name: "negative sentinel is not an output cap", limit: new(-1), calls: 2},
		{name: "zero is not a positive output cap", limit: new(0), calls: 2},
		{name: "every length cut off", calls: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := evidenceAssistant(t, 1, 65536, 4096)
			a.cfg.Features.Compaction.MaxTokens = 20000
			a.agent.Model.Generation = &generation.Generation{MaxOutputTokens: tc.limit}
			readHistory(t, a, evidence(40000))
			before := slices.Clone(a.builder.History())
			seen := probeUsage(a, func(int, request.Request) (message.Message, usage.Usage, error) {
				return cutOff, usage.Usage{Output: 1000}, nil
			})

			err := a.CompactWith(t.Context(), true, 1)
			if !errors.Is(err, ErrCompactionSummaryCutOff) || errors.Is(err, providers.ErrContextExhausted) || len(*seen) != tc.calls {
				t.Fatalf("calls=%d err=%v", len(*seen), err)
			}

			if !reflect.DeepEqual(before, a.builder.History()) {
				t.Fatal("a cut-off summary changed the history")
			}

			*seen = nil
			if err := a.RecoverCompaction(t.Context(), 1); !errors.Is(err, ErrCompactionExhausted) || len(*seen) != tc.calls {
				t.Fatalf("recovery calls=%d err=%v; a cut-off must not shrink the preserved tail", len(*seen), err)
			}
		})
	}

	t.Run("ordinary chat keeps a cut-off response", func(t *testing.T) {
		a := evidenceAssistant(t, 1, 200)
		a.resolved.model = &model.Model{Name: "test"}
		a.agent.Provider = compactionRequestProbe{Provider: a.agent.Provider, chat: func(request.Request) (message.Message, usage.Usage, error) {
			return message.Message{Role: roles.Assistant, Content: "Partial answer", Finish: message.FinishLength}, usage.Usage{}, nil
		}}

		if err := a.ProcessInput(t.Context(), "Hello"); err != nil {
			t.Fatal(err)
		}

		if last := a.builder.History().WithoutInternalMessages(); last[len(last)-1].Content != "Partial answer" {
			t.Fatalf("ordinary chat changed: %q", last[len(last)-1].Content)
		}
	})
}
