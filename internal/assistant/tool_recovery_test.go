package assistant

import (
	"fmt"
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/aura/pkg/llm/tool/call"
)

func TestArgumentFailureRemainsAfterCorrection(t *testing.T) {
	a := selectionAssistant(t)
	if err := a.MergeFeatures(config.Features{ToolExecution: config.ToolExecution{
		Bindings: tool.Bindings{"Bash": {"timeout_ms": 15000}},
	}}); err != nil {
		t.Fatal(err)
	}
	turn := 0
	a.agent.Provider = toolFilterProvider{Provider: a.agent.Provider, respond: func(req request.Request) message.Message {
		turn++
		if turn == 1 {
			return message.Message{Calls: []call.Call{{ID: "bad", Name: "Bash", Arguments: map[string]any{"command": false}}}}
		}
		if turn == 2 {
			return message.Message{Calls: []call.Call{{ID: "corrected", Name: "Bash", Arguments: map[string]any{"command": "printf recovered"}}}}
		}
		for _, msg := range req.Messages {
			if msg.ToolCallID == "bad" && strings.Contains(msg.Content, "expected string") {
				return message.Message{Content: "Finished with correction retained."}
			}
		}
		t.Error("argument failure vanished after one model turn")
		return message.Message{Content: "Finished."}
	}}
	if err := a.ProcessInput(t.Context(), "Execute the tool sequence."); err != nil {
		t.Fatal(err)
	}
	if turn != 3 {
		t.Fatal(turn)
	}
}

func TestRecoveryMessagesSurviveLaterTurns(t *testing.T) {
	for _, malformed := range []bool{false, true} {
		t.Run(fmt.Sprint(malformed), func(t *testing.T) {
			a := selectionAssistant(t)
			turn := 0
			a.agent.Provider = toolFilterProvider{Provider: a.agent.Provider, respond: func(req request.Request) message.Message {
				turn++
				if turn == 1 {
					if malformed {
						return message.Message{Content: "<tool_call>\n<function=Bash=\n</parameter>"}
					}
					return message.Message{Calls: []call.Call{{ID: "stringified", Name: "Bash", Arguments: map[string]any{"command": "printf corrected", "timeout_ms": "15000"}}}}
				}
				if turn == 2 {
					return message.Message{Calls: []call.Call{{ID: "later", Name: "Bash", Arguments: map[string]any{"command": "printf later"}}}}
				}
				found := false
				for i, msg := range req.Messages {
					if i > 0 && msg.Role == roles.System {
						t.Fatal("tool recovery introduced a mid-conversation system prompt")
					}
					if strings.HasPrefix(msg.Content, "[Aura tool recovery]") && msg.Role == roles.User {
						found = true
					}
				}
				if !found {
					t.Error("recovery feedback vanished after a later tool turn")
				}
				return message.Message{Content: "Done."}
			}}
			if err := a.ProcessInput(t.Context(), "Exercise recovery."); err != nil {
				t.Fatal(err)
			}
			if turn != 3 {
				t.Fatalf("unexpected model turns: %d", turn)
			}
		})
	}
}

func TestMalformedTextCallContinuesInsteadOfFinishing(t *testing.T) {
	for _, repeat := range []bool{false, true} {
		t.Run(fmt.Sprint(repeat), func(t *testing.T) {
			a := selectionAssistant(t)
			turn := 0
			a.agent.Provider = toolFilterProvider{Provider: a.agent.Provider, respond: func(req request.Request) message.Message {
				turn++
				if repeat || turn == 1 {
					return message.Message{Content: "<tool_call>\n<function=Bash=\n</parameter>"}
				}
				return message.Message{Content: "Recovered."}
			}}
			err := a.ProcessInput(t.Context(), "Complete the task.")
			if repeat {
				if err == nil || turn != maxParseRetries+1 {
					t.Fatalf("repeated malformed calls reported success or retried forever: turn=%d, err=%v", turn, err)
				}
			} else if err != nil || turn != 2 {
				t.Fatalf("turn=%d, err=%v", turn, err)
			}
		})
	}
}
