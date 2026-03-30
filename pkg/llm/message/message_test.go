package message_test

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/part"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/tool/call"
)

func TestRoleChecks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		role        roles.Role
		isUser      bool
		isAssistant bool
		isTool      bool
		isSystem    bool
	}{
		{name: "user role", role: roles.User, isUser: true},
		{name: "assistant role", role: roles.Assistant, isAssistant: true},
		{name: "tool role", role: roles.Tool, isTool: true},
		{name: "system role", role: roles.System, isSystem: true},
		{name: "synthetic role", role: roles.Synthetic},
		{name: "empty role"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := message.Message{Role: tt.role}

			if got := m.IsUser(); got != tt.isUser {
				t.Errorf("IsUser() = %v, want %v", got, tt.isUser)
			}

			if got := m.IsAssistant(); got != tt.isAssistant {
				t.Errorf("IsAssistant() = %v, want %v", got, tt.isAssistant)
			}

			if got := m.IsTool(); got != tt.isTool {
				t.Errorf("IsTool() = %v, want %v", got, tt.isTool)
			}

			if got := m.IsSystem(); got != tt.isSystem {
				t.Errorf("IsSystem() = %v, want %v", got, tt.isSystem)
			}
		})
	}
}

func TestTypePredicates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		typ           message.Type
		isSynthetic   bool
		isEphemeral   bool
		isDisplayOnly bool
		isBookmark    bool
		isMetadata    bool
		isInternal    bool
	}{
		{name: "Normal", typ: message.Normal},
		{name: "Synthetic", typ: message.Synthetic, isSynthetic: true},
		{name: "Ephemeral", typ: message.Ephemeral, isEphemeral: true},
		{name: "DisplayOnly", typ: message.DisplayOnly, isDisplayOnly: true, isInternal: true},
		{name: "Bookmark", typ: message.Bookmark, isBookmark: true, isInternal: true},
		{name: "Metadata", typ: message.Metadata, isMetadata: true, isInternal: true},
		{name: "zero value", typ: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := message.Message{Type: tt.typ}

			if got := m.IsSynthetic(); got != tt.isSynthetic {
				t.Errorf("IsSynthetic() = %v, want %v", got, tt.isSynthetic)
			}

			if got := m.IsEphemeral(); got != tt.isEphemeral {
				t.Errorf("IsEphemeral() = %v, want %v", got, tt.isEphemeral)
			}

			if got := m.IsDisplayOnly(); got != tt.isDisplayOnly {
				t.Errorf("IsDisplayOnly() = %v, want %v", got, tt.isDisplayOnly)
			}

			if got := m.IsBookmark(); got != tt.isBookmark {
				t.Errorf("IsBookmark() = %v, want %v", got, tt.isBookmark)
			}

			if got := m.IsMetadata(); got != tt.isMetadata {
				t.Errorf("IsMetadata() = %v, want %v", got, tt.isMetadata)
			}

			if got := m.IsInternal(); got != tt.isInternal {
				t.Errorf("IsInternal() = %v, want %v", got, tt.isInternal)
			}
		})
	}
}

func TestRender(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		msg      message.Message
		contains []string
		absent   []string
	}{
		{
			name:     "user message",
			msg:      message.Message{Role: roles.User, Content: "hello world"},
			contains: []string{"[User]: hello world"},
		},
		{
			name:     "assistant with content only",
			msg:      message.Message{Role: roles.Assistant, Content: "here is the answer"},
			contains: []string{"[Assistant]: here is the answer"},
			absent:   []string{"[Assistant Thinking]"},
		},
		{
			name: "assistant with thinking and content",
			msg: message.Message{
				Role:     roles.Assistant,
				Thinking: "let me reason about this",
				Content:  "the answer is 42",
			},
			contains: []string{
				"[Assistant Thinking]: let me reason about this",
				"[Assistant]: the answer is 42",
			},
		},
		{
			name: "assistant with thinking only, no content",
			msg: message.Message{
				Role:     roles.Assistant,
				Thinking: "just thinking",
			},
			contains: []string{"[Assistant Thinking]: just thinking"},
			absent:   []string{"[Assistant]:"},
		},
		{
			name: "tool result message",
			msg: message.Message{
				Role:     roles.Tool,
				ToolName: "read_file",
				Content:  "file contents here",
			},
			contains: []string{"← read_file result:", "file contents here"},
		},
		{
			name:     "system message",
			msg:      message.Message{Role: roles.System, Content: "you are helpful"},
			contains: []string{"[System]: you are helpful"},
		},
		{
			name: "assistant with tool calls",
			msg: message.Message{
				Role: roles.Assistant,
				Calls: []call.Call{
					{Name: "bash", Arguments: map[string]any{"cmd": "ls"}},
				},
			},
			contains: []string{"bash"},
		},
		{
			name:     "bookmark renders as divider",
			msg:      message.Message{Role: roles.System, Content: "Context compacted", Type: message.Bookmark},
			contains: []string{"---", "Context compacted"},
		},
		{
			name: "metadata renders as empty",
			msg:  message.Message{Role: roles.System, Content: `{"cost":0.03}`, Type: message.Metadata},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.msg.Render()

			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("Render() = %q, want it to contain %q", got, want)
				}
			}

			for _, unwanted := range tt.absent {
				if strings.Contains(got, unwanted) {
					t.Errorf("Render() = %q, want it NOT to contain %q", got, unwanted)
				}
			}
		})
	}
}

func TestForLog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		msg      message.Message
		contains []string
		absent   []string
	}{
		{
			name: "assistant with thinking",
			msg: message.Message{
				Role:     roles.Assistant,
				Thinking: "my reasoning",
				Content:  "my answer",
			},
			contains: []string{
				"[assistant (thinking)] my reasoning",
				"[assistant (content)] my answer",
			},
		},
		{
			name: "assistant without thinking",
			msg: message.Message{
				Role:    roles.Assistant,
				Content: "just content",
			},
			contains: []string{"[assistant (content)] just content"},
			absent:   []string{"thinking"},
		},
		{
			name: "tool message",
			msg: message.Message{
				Role:     roles.Tool,
				ToolName: "read_file",
				Content:  "result data",
			},
			contains: []string{"[tool] read_file: result data"},
		},
		{
			name: "user message uses default branch",
			msg: message.Message{
				Role:    roles.User,
				Content: "user input",
			},
			contains: []string{"[user] user input"},
		},
		{
			name: "system message uses default branch",
			msg: message.Message{
				Role:    roles.System,
				Content: "system prompt",
			},
			contains: []string{"[system] system prompt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.msg.ForLog()

			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("ForLog() = %q, want it to contain %q", got, want)
				}
			}

			for _, unwanted := range tt.absent {
				if strings.Contains(got, unwanted) {
					t.Errorf("ForLog() = %q, want it NOT to contain %q", got, unwanted)
				}
			}
		})
	}
}

func TestForTranscript(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		msg       message.Message
		contains  []string
		wantEmpty bool
	}{
		{
			name:     "user message",
			msg:      message.Message{Role: roles.User, Content: "what is 2+2"},
			contains: []string{"[User]: what is 2+2"},
		},
		{
			name:     "assistant with content",
			msg:      message.Message{Role: roles.Assistant, Content: "it is 4"},
			contains: []string{"[Assistant]: it is 4"},
		},
		{
			name: "assistant with tool calls",
			msg: message.Message{
				Role: roles.Assistant,
				Calls: []call.Call{
					{Name: "calculator", Arguments: map[string]any{"expr": "2+2"}},
				},
			},
			contains: []string{"calculator"},
		},
		{
			name: "tool result",
			msg: message.Message{
				Role:     roles.Tool,
				ToolName: "read_file",
				Content:  "line1\nline2",
			},
			contains: []string{"← read_file result:", "line1"},
		},
		{
			name:      "system message is skipped",
			msg:       message.Message{Role: roles.System, Content: "system prompt"},
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.msg.ForTranscript()

			if tt.wantEmpty {
				if got != "" {
					t.Errorf("ForTranscript() = %q, want empty string", got)
				}

				return
			}

			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("ForTranscript() = %q, want it to contain %q", got, want)
				}
			}
		})
	}
}

func TestDataURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		data   []byte
		prefix string
	}{
		{
			name:   "small byte slice",
			data:   []byte{0xFF, 0xD8, 0xFF, 0xE0},
			prefix: "data:image/jpeg;base64,",
		},
		{
			name:   "empty data",
			data:   []byte{},
			prefix: "data:image/jpeg;base64,",
		},
		{
			name:   "single byte",
			data:   []byte{0xAB},
			prefix: "data:image/jpeg;base64,",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			img := message.Image{Data: tt.data}
			got := img.DataURL()

			if !strings.HasPrefix(got, tt.prefix) {
				t.Errorf("DataURL() = %q, want prefix %q", got, tt.prefix)
			}

			// Verify the base64 portion decodes back to the original data.
			encoded := strings.TrimPrefix(got, tt.prefix)

			decoded, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				t.Fatalf("base64 decode failed: %v", err)
			}

			if len(decoded) != len(tt.data) {
				t.Errorf("decoded length = %d, want %d", len(decoded), len(tt.data))
			}

			for i, b := range tt.data {
				if decoded[i] != b {
					t.Errorf("decoded[%d] = %x, want %x", i, decoded[i], b)
				}
			}
		})
	}
}

func TestUnmarshalJSONUser(t *testing.T) {
	t.Parallel()

	data := `{"Role":"user","Content":"hello"}`

	var m message.Message
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if m.Role != roles.User {
		t.Errorf("Role = %q, want %q", m.Role, roles.User)
	}

	if m.Content != "hello" {
		t.Errorf("Content = %q, want %q", m.Content, "hello")
	}

	if len(m.Parts) != 1 {
		t.Fatalf("len(Parts) = %d, want 1", len(m.Parts))
	}

	if m.Parts[0].Type != part.Content {
		t.Errorf("Parts[0].Type = %q, want %q", m.Parts[0].Type, part.Content)
	}

	if m.Parts[0].Text != "hello" {
		t.Errorf("Parts[0].Text = %q, want %q", m.Parts[0].Text, "hello")
	}
}

func TestUnmarshalJSONSystem(t *testing.T) {
	t.Parallel()

	data := `{"Role":"system","Content":"be helpful"}`

	var m message.Message
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if m.Role != roles.System {
		t.Errorf("Role = %q, want %q", m.Role, roles.System)
	}

	if len(m.Parts) != 1 {
		t.Fatalf("len(Parts) = %d, want 1", len(m.Parts))
	}

	if m.Parts[0].Type != part.Content {
		t.Errorf("Parts[0].Type = %q, want %q", m.Parts[0].Type, part.Content)
	}
}

func TestUnmarshalJSONAssistantFull(t *testing.T) {
	t.Parallel()

	data := `{"Role":"assistant","Content":"answer","Thinking":"reasoning","calls":[{"ID":"c1","Name":"bash","Arguments":{"cmd":"ls"}},{"ID":"c2","Name":"read","Arguments":{"path":"/"}}]}`

	var m message.Message
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if m.Role != roles.Assistant {
		t.Errorf("Role = %q, want %q", m.Role, roles.Assistant)
	}

	// Parts order: Thinking, Content, Tool, Tool
	if len(m.Parts) != 4 {
		t.Fatalf("len(Parts) = %d, want 4", len(m.Parts))
	}

	if m.Parts[0].Type != part.Thinking {
		t.Errorf("Parts[0].Type = %q, want %q", m.Parts[0].Type, part.Thinking)
	}

	if m.Parts[0].Text != "reasoning" {
		t.Errorf("Parts[0].Text = %q, want %q", m.Parts[0].Text, "reasoning")
	}

	if m.Parts[1].Type != part.Content {
		t.Errorf("Parts[1].Type = %q, want %q", m.Parts[1].Type, part.Content)
	}

	if m.Parts[2].Type != part.Tool {
		t.Errorf("Parts[2].Type = %q, want %q", m.Parts[2].Type, part.Tool)
	}

	if m.Parts[2].Call.Name != "bash" {
		t.Errorf("Parts[2].Call.Name = %q, want %q", m.Parts[2].Call.Name, "bash")
	}

	if m.Parts[3].Call.Name != "read" {
		t.Errorf("Parts[3].Call.Name = %q, want %q", m.Parts[3].Call.Name, "read")
	}
}

func TestUnmarshalJSONNewTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		json    string
		wantTyp message.Type
	}{
		{
			name:    "DisplayOnly type round-trips",
			json:    `{"Role":"user","Content":"notice","type":3}`,
			wantTyp: message.DisplayOnly,
		},
		{
			name:    "Bookmark type round-trips",
			json:    `{"Role":"system","Content":"compacted","type":4}`,
			wantTyp: message.Bookmark,
		},
		{
			name:    "Metadata type round-trips",
			json:    `{"Role":"system","Content":"{\"cost\":0.03}","type":5}`,
			wantTyp: message.Metadata,
		},
		{
			name:    "Normal type omitted in JSON",
			json:    `{"Role":"user","Content":"hello"}`,
			wantTyp: message.Normal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var m message.Message
			if err := json.Unmarshal([]byte(tt.json), &m); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}

			if m.Type != tt.wantTyp {
				t.Errorf("Type = %d, want %d", m.Type, tt.wantTyp)
			}
		})
	}
}

func TestReconstructPartsOnlyThinking(t *testing.T) {
	t.Parallel()

	m := message.Message{
		Role:     roles.Assistant,
		Thinking: "deep thought",
	}
	m.ReconstructParts()

	if len(m.Parts) != 1 {
		t.Fatalf("len(Parts) = %d, want 1", len(m.Parts))
	}

	if m.Parts[0].Type != part.Thinking {
		t.Errorf("Parts[0].Type = %q, want %q", m.Parts[0].Type, part.Thinking)
	}

	if m.Parts[0].Text != "deep thought" {
		t.Errorf("Parts[0].Text = %q, want %q", m.Parts[0].Text, "deep thought")
	}
}

func TestReconstructPartsOnlyCalls(t *testing.T) {
	t.Parallel()

	m := message.Message{
		Role: roles.Assistant,
		Calls: []call.Call{
			{ID: "c1", Name: "bash", Arguments: map[string]any{"cmd": "ls"}},
		},
	}
	m.ReconstructParts()

	if len(m.Parts) != 1 {
		t.Fatalf("len(Parts) = %d, want 1", len(m.Parts))
	}

	if m.Parts[0].Type != part.Tool {
		t.Errorf("Parts[0].Type = %q, want %q", m.Parts[0].Type, part.Tool)
	}

	if m.Parts[0].Call == nil {
		t.Fatal("Parts[0].Call = nil, want non-nil")
	}

	if m.Parts[0].Call.Name != "bash" {
		t.Errorf("Parts[0].Call.Name = %q, want %q", m.Parts[0].Call.Name, "bash")
	}
}

func TestReconstructPartsToolRole(t *testing.T) {
	t.Parallel()

	m := message.Message{
		Role:    roles.Tool,
		Content: "tool output",
	}
	m.ReconstructParts()

	if len(m.Parts) != 1 {
		t.Fatalf("len(Parts) = %d, want 1", len(m.Parts))
	}

	if m.Parts[0].Type != part.Content {
		t.Errorf("Parts[0].Type = %q, want %q", m.Parts[0].Type, part.Content)
	}

	if m.Parts[0].Text != "tool output" {
		t.Errorf("Parts[0].Text = %q, want %q", m.Parts[0].Text, "tool output")
	}
}

func TestUnmarshalJSONRoundTrip(t *testing.T) {
	t.Parallel()

	orig := message.Message{
		Role:     roles.Assistant,
		Content:  "the answer",
		Thinking: "let me think",
		Calls: []call.Call{
			{ID: "c1", Name: "bash", Arguments: map[string]any{"cmd": "ls"}},
		},
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got message.Message
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got.Role != orig.Role {
		t.Errorf("Role = %q, want %q", got.Role, orig.Role)
	}

	if got.Content != orig.Content {
		t.Errorf("Content = %q, want %q", got.Content, orig.Content)
	}

	if got.Thinking != orig.Thinking {
		t.Errorf("Thinking = %q, want %q", got.Thinking, orig.Thinking)
	}

	if len(got.Calls) != 1 {
		t.Fatalf("len(Calls) = %d, want 1", len(got.Calls))
	}

	if got.Calls[0].Name != "bash" {
		t.Errorf("Calls[0].Name = %q, want %q", got.Calls[0].Name, "bash")
	}

	// Parts must be rebuilt by UnmarshalJSON.
	if len(got.Parts) != 3 {
		t.Fatalf("len(Parts) = %d, want 3 (thinking+content+tool)", len(got.Parts))
	}
}
