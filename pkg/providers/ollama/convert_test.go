package ollama_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/ollama/ollama/api"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/thinking"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/aura/pkg/llm/tool/call"
	"github.com/idelchi/aura/pkg/providers/ollama"
)

func TestToAPIMessageUser(t *testing.T) {
	t.Parallel()

	msg := message.Message{
		Role:    roles.User,
		Content: "hello world",
	}

	got := ollama.ToAPIMessage(msg)

	if got.Role != "user" {
		t.Errorf("Role = %q, want %q", got.Role, "user")
	}

	if got.Content != "hello world" {
		t.Errorf("Content = %q, want %q", got.Content, "hello world")
	}

	if len(got.ToolCalls) != 0 {
		t.Errorf("ToolCalls = %v, want empty", got.ToolCalls)
	}
}

func TestToAPIMessageAssistant(t *testing.T) {
	t.Parallel()

	msg := message.Message{
		Role:    roles.Assistant,
		Content: "I will call a tool",
		Calls: []call.Call{
			{ID: "call-1", Name: "my_tool", Arguments: map[string]any{"key": "val"}},
		},
	}

	got := ollama.ToAPIMessage(msg)

	if got.Role != "assistant" {
		t.Errorf("Role = %q, want %q", got.Role, "assistant")
	}

	if len(got.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(got.ToolCalls))
	}

	if got.ToolCalls[0].ID != "call-1" {
		t.Errorf("ToolCalls[0].ID = %q, want %q", got.ToolCalls[0].ID, "call-1")
	}

	if got.ToolCalls[0].Function.Name != "my_tool" {
		t.Errorf("ToolCalls[0].Function.Name = %q, want %q", got.ToolCalls[0].Function.Name, "my_tool")
	}
}

func TestToAPIMessageTool(t *testing.T) {
	t.Parallel()

	msg := message.Message{
		Role:       roles.Tool,
		Content:    "tool result",
		ToolName:   "my_tool",
		ToolCallID: "call-1",
	}

	got := ollama.ToAPIMessage(msg)

	if got.Role != "tool" {
		t.Errorf("Role = %q, want %q", got.Role, "tool")
	}

	if got.ToolName != "my_tool" {
		t.Errorf("ToolName = %q, want %q", got.ToolName, "my_tool")
	}

	if got.ToolCallID != "call-1" {
		t.Errorf("ToolCallID = %q, want %q", got.ToolCallID, "call-1")
	}
}

func TestToAPIMessages(t *testing.T) {
	t.Parallel()

	msgs := message.New(
		message.Message{Role: roles.System, Content: "system prompt"},
		message.Message{Role: roles.User, Content: "first user"},
		message.Message{Role: roles.Assistant, Content: "first assistant"},
	)

	got := ollama.ToAPIMessages(msgs)

	if len(got) != 3 {
		t.Fatalf("len(result) = %d, want 3", len(got))
	}

	if got[0].Role != "system" {
		t.Errorf("got[0].Role = %q, want %q", got[0].Role, "system")
	}

	if got[1].Role != "user" {
		t.Errorf("got[1].Role = %q, want %q", got[1].Role, "user")
	}

	if got[2].Role != "assistant" {
		t.Errorf("got[2].Role = %q, want %q", got[2].Role, "assistant")
	}

	// order preserved
	if got[1].Content != "first user" {
		t.Errorf("got[1].Content = %q, want %q", got[1].Content, "first user")
	}
}

func TestToCallRoundTrip(t *testing.T) {
	t.Parallel()

	original := []call.Call{
		{ID: "id-1", Name: "tool_a", Arguments: map[string]any{"x": "1", "y": float64(2)}},
		{ID: "id-2", Name: "tool_b", Arguments: map[string]any{"flag": true}},
	}

	apiCalls := ollama.ToAPIToolCalls(original)

	roundTripped := make([]call.Call, len(apiCalls))
	for i, tc := range apiCalls {
		converted, err := ollama.ToCall(tc)
		if err != nil {
			t.Fatalf("ToCall[%d]: %v", i, err)
		}

		roundTripped[i] = converted
	}

	if len(roundTripped) != len(original) {
		t.Fatalf("len = %d, want %d", len(roundTripped), len(original))
	}

	for i, orig := range original {
		rt := roundTripped[i]
		if rt.ID != orig.ID {
			t.Errorf("[%d] ID = %q, want %q", i, rt.ID, orig.ID)
		}

		if rt.Name != orig.Name {
			t.Errorf("[%d] Name = %q, want %q", i, rt.Name, orig.Name)
		}

		for k, wantV := range orig.Arguments {
			gotV, ok := rt.Arguments[k]
			if !ok {
				t.Errorf("[%d] Arguments[%q] missing", i, k)

				continue
			}

			// ToAPIToolCalls serialises via args.Set and ToCall deserialises via ToMap,
			// so compare using fmt.Sprint to handle numeric type differences (e.g. float64 vs int).
			if wantS, gotS := fmt.Sprint(wantV), fmt.Sprint(gotV); wantS != gotS {
				t.Errorf("[%d] Arguments[%q] = %v, want %v", i, k, gotV, wantV)
			}
		}
	}
}

func TestToMessage(t *testing.T) {
	t.Parallel()

	args := api.NewToolCallFunctionArguments()
	args.Set("param", "value")

	apiMsg := api.Message{
		Role:     "assistant",
		Content:  "response content",
		Thinking: "internal reasoning",
		ToolCalls: []api.ToolCall{
			{
				ID: "tc-1",
				Function: api.ToolCallFunction{
					Name:      "some_tool",
					Arguments: args,
				},
			},
		},
	}

	got, err := ollama.ToMessage(apiMsg)
	if err != nil {
		t.Fatalf("ToMessage: %v", err)
	}

	if got.Role != roles.Assistant {
		t.Errorf("Role = %q, want %q", got.Role, roles.Assistant)
	}

	if got.Content != "response content" {
		t.Errorf("Content = %q, want %q", got.Content, "response content")
	}

	if got.Thinking != "internal reasoning" {
		t.Errorf("Thinking = %q, want %q", got.Thinking, "internal reasoning")
	}

	if len(got.Calls) != 1 {
		t.Fatalf("len(Calls) = %d, want 1", len(got.Calls))
	}

	if got.Calls[0].ID != "tc-1" {
		t.Errorf("Calls[0].ID = %q, want %q", got.Calls[0].ID, "tc-1")
	}

	if got.Calls[0].Name != "some_tool" {
		t.Errorf("Calls[0].Name = %q, want %q", got.Calls[0].Name, "some_tool")
	}
}

func TestToTools(t *testing.T) {
	t.Parallel()

	schemas := []tool.Schema{
		{
			Name:        "search",
			Description: "search the web",
			Parameters: tool.Parameters{
				Type: "object",
				Properties: map[string]tool.Property{
					"query": {Type: "string", Description: "search query"},
				},
				Required: []string{"query"},
			},
		},
		{
			Name:        "calculator",
			Description: "do math",
			Parameters: tool.Parameters{
				Type:       "object",
				Properties: map[string]tool.Property{},
				Required:   []string{},
			},
		},
	}

	got := ollama.ToTools(schemas)

	if len(got) != 2 {
		t.Fatalf("len(tools) = %d, want 2", len(got))
	}

	if got[0].Function.Name != "search" {
		t.Errorf("got[0].Function.Name = %q, want %q", got[0].Function.Name, "search")
	}

	if got[1].Function.Name != "calculator" {
		t.Errorf("got[1].Function.Name = %q, want %q", got[1].Function.Name, "calculator")
	}

	if got[0].Function.Description != "search the web" {
		t.Errorf("got[0].Function.Description = %q, want %q", got[0].Function.Description, "search the web")
	}
}

func TestToChatRequest(t *testing.T) {
	t.Parallel()

	client, err := ollama.New("http://localhost", "", 0, 0)
	if err != nil {
		t.Fatalf("ollama.New: %v", err)
	}

	req := request.Request{
		Model: model.Model{Name: "llama3.2"},
		Messages: message.New(
			message.Message{Role: roles.User, Content: "hi"},
		),
		Tools: []tool.Schema{
			{
				Name:        "ping",
				Description: "ping a host",
				Parameters: tool.Parameters{
					Type:       "object",
					Properties: map[string]tool.Property{},
					Required:   []string{},
				},
			},
		},
	}

	got, err := client.ToChatRequest(req)
	if err != nil {
		t.Fatalf("ToChatRequest: %v", err)
	}

	if got.Model != "llama3.2" {
		t.Errorf("Model = %q, want %q", got.Model, "llama3.2")
	}

	if len(got.Messages) != 1 {
		t.Errorf("len(Messages) = %d, want 1", len(got.Messages))
	}

	if got.Messages[0].Content != "hi" {
		t.Errorf("Messages[0].Content = %q, want %q", got.Messages[0].Content, "hi")
	}

	if len(got.Tools) != 1 {
		t.Errorf("len(Tools) = %d, want 1", len(got.Tools))
	}

	if got.Tools[0].Function.Name != "ping" {
		t.Errorf("Tools[0].Function.Name = %q, want %q", got.Tools[0].Function.Name, "ping")
	}
}

func TestToChatRequestThinkingEnabled(t *testing.T) {
	t.Parallel()

	client, err := ollama.New("http://localhost", "", 0, 0)
	if err != nil {
		t.Fatalf("ollama.New: %v", err)
	}

	think := thinking.NewValue(true)
	req := request.Request{
		Model: model.Model{Name: "llama3.2"},
		Messages: message.New(
			message.Message{Role: roles.User, Content: "hi"},
		),
		Think: think.Ptr(),
	}

	got, err := client.ToChatRequest(req)
	if err != nil {
		t.Fatalf("ToChatRequest: %v", err)
	}

	if got.Think == nil {
		t.Fatal("Think = nil, want non-nil when thinking enabled")
	}
}

func TestToChatRequestContextLength(t *testing.T) {
	t.Parallel()

	client, err := ollama.New("http://localhost", "", 0, 0)
	if err != nil {
		t.Fatalf("ollama.New: %v", err)
	}

	req := request.Request{
		Model: model.Model{Name: "llama3.2"},
		Messages: message.New(
			message.Message{Role: roles.User, Content: "hi"},
		),
		ContextLength: 8192,
	}

	got, err := client.ToChatRequest(req)
	if err != nil {
		t.Fatalf("ToChatRequest: %v", err)
	}

	numCtx, ok := got.Options["num_ctx"]
	if !ok {
		t.Fatal("Options[num_ctx] not set, want 8192")
	}

	if numCtx != 8192 {
		t.Errorf("Options[num_ctx] = %v, want 8192", numCtx)
	}
}

func TestToChatRequestContextLengthZero(t *testing.T) {
	t.Parallel()

	client, err := ollama.New("http://localhost", "", 0, 0)
	if err != nil {
		t.Fatalf("ollama.New: %v", err)
	}

	req := request.Request{
		Model: model.Model{Name: "llama3.2"},
		Messages: message.New(
			message.Message{Role: roles.User, Content: "hi"},
		),
		ContextLength: 0,
	}

	got, err := client.ToChatRequest(req)
	if err != nil {
		t.Fatalf("ToChatRequest: %v", err)
	}

	if _, ok := got.Options["num_ctx"]; ok {
		t.Error("Options[num_ctx] set for zero context length, want absent")
	}
}

func TestToChatRequestKeepAlive(t *testing.T) {
	t.Parallel()

	client, err := ollama.New("http://localhost", "", 5*time.Minute, 0)
	if err != nil {
		t.Fatalf("ollama.New: %v", err)
	}

	req := request.Request{
		Model: model.Model{Name: "llama3.2"},
		Messages: message.New(
			message.Message{Role: roles.User, Content: "hi"},
		),
	}

	got, err := client.ToChatRequest(req)
	if err != nil {
		t.Fatalf("ToChatRequest: %v", err)
	}

	if got.KeepAlive == nil {
		t.Fatal("KeepAlive = nil, want non-nil when keepAlive > 0")
	}

	if got.KeepAlive.Duration != 5*time.Minute {
		t.Errorf("KeepAlive.Duration = %v, want %v", got.KeepAlive.Duration, 5*time.Minute)
	}
}
