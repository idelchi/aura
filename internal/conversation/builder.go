package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/part"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/tool/call"
	"github.com/idelchi/aura/sdk"
)

// EventSink is a function that receives conversation events.
// Nil is safe — emit() guards all calls.
type EventSink func(event ui.Event)

// Builder manages conversation history and emits UI events.
//
// Message lifecycle:
//
//	StartAssistant() → creates current message, emits MessageStarted
//	AppendContent/AppendThinking → appends to current, emits MessagePartAdded/Updated
//	AddToolCall/StartToolCall/CompleteToolCall → manages tool call parts
//	FinalizeAssistant() → commits current to history, emits MessageFinalized
//
// current is nil between messages. Methods that require current
// log a warning and return if called outside the Start/Finalize lifecycle.
type Builder struct {
	history  message.Messages // Conversation history (sent to provider)
	current  *message.Message
	sink     EventSink
	msgID    atomic.Uint64
	estimate func(context.Context, string) int // Token estimation function (context-aware, supports native).
}

// NewBuilder creates a new conversation builder with the given event sink, system prompt,
// pre-computed system token count, and estimate function.
func NewBuilder(
	sink EventSink,
	systemPrompt string,
	systemTokens int,
	estimate func(context.Context, string) int,
) *Builder {
	return &Builder{
		history: message.New(message.Message{
			Role:    roles.System,
			Content: systemPrompt,
			Tokens:  message.Tokens{Total: systemTokens},
		}),
		sink:     sink,
		estimate: estimate,
	}
}

// emit sends an event to the sink. No-op if sink is nil.
func (b *Builder) emit(event ui.Event) {
	if b.sink != nil {
		b.sink(event)
	}
}

// Estimate returns the token count for text using the configured estimator.
func (b *Builder) Estimate(ctx context.Context, text string) int {
	return b.estimate(ctx, text)
}

// History returns the message history for API calls.
func (b *Builder) History() message.Messages {
	return b.history
}

// NextID generates a unique message ID.
func (b *Builder) NextID() string {
	id := b.msgID.Add(1)

	return fmt.Sprintf("msg-%d", id)
}

// AddUserMessage adds a completed user message and emits MessageAdded.
// When estimatedTokens > 0, it is used directly (avoids re-estimation of already-estimated text).
func (b *Builder) AddUserMessage(ctx context.Context, text string, estimatedTokens int) {
	now := time.Now()

	msg := message.Message{
		ID:        b.NextID(),
		Role:      roles.User,
		Content:   text,
		CreatedAt: now,
		Parts: []part.Part{
			{Type: part.Content, Text: text},
		},
	}

	tokensVal := estimatedTokens
	if tokensVal == 0 {
		tokensVal = b.Estimate(ctx, text)
	}

	b.history.Add(
		message.Message{Role: roles.User, Content: text, CreatedAt: now, Tokens: message.Tokens{Total: tokensVal}},
	)

	b.emit(ui.MessageAdded{Message: msg})
}

// AddUserMessageWithImages adds a completed user message with images and emits MessageAdded.
// Images flow through to providers via their convert layers
// (Ollama: raw bytes, OpenRouter/LlamaCPP: base64 data URLs).
// When estimatedTokens > 0, it is used directly (avoids re-estimation).
func (b *Builder) AddUserMessageWithImages(
	ctx context.Context,
	text string,
	images message.Images,
	estimatedTokens int,
) {
	now := time.Now()

	msg := message.Message{
		ID:        b.NextID(),
		Role:      roles.User,
		Content:   text,
		Images:    images,
		CreatedAt: now,
		Parts: []part.Part{
			{Type: part.Content, Text: text},
		},
	}

	tokensVal := estimatedTokens
	if tokensVal == 0 {
		tokensVal = b.Estimate(ctx, text)
	}

	b.history.Add(
		message.Message{
			Role:      roles.User,
			Content:   text,
			Images:    images,
			CreatedAt: now,
			Tokens:    message.Tokens{Total: tokensVal},
		},
	)

	b.emit(ui.MessageAdded{Message: msg})
}

// StartAssistant begins a new assistant message and emits MessageStarted.
func (b *Builder) StartAssistant() {
	b.current = &message.Message{
		ID:    b.NextID(),
		Role:  roles.Assistant,
		Parts: []part.Part{},
	}

	b.emit(ui.MessageStarted{MessageID: b.current.ID})
}

// AppendContent appends content text to the current assistant message.
// Merges with the last content part if present, otherwise creates a new part.
func (b *Builder) AppendContent(content string) {
	if content == "" {
		return
	}

	if b.current == nil {
		debug.Log("[builder] AppendContent called without active message")

		return
	}

	// Try to merge with last content part
	if len(b.current.Parts) > 0 {
		last := &b.current.Parts[len(b.current.Parts)-1]
		if last.IsContent() {
			last.Text += content

			b.emit(ui.MessagePartUpdated{
				MessageID: b.current.ID,
				PartIndex: len(b.current.Parts) - 1,
				Part:      *last,
			})

			return
		}
	}

	// Add new content part
	p := part.Part{Type: part.Content, Text: content}

	b.current.Parts = append(b.current.Parts, p)

	b.emit(ui.MessagePartAdded{
		MessageID: b.current.ID,
		Part:      p,
	})
}

// AppendThinking appends thinking text to the current assistant message.
func (b *Builder) AppendThinking(thinking string) {
	if thinking == "" {
		return
	}

	if b.current == nil {
		debug.Log("[builder] AppendThinking called without active message")

		return
	}

	// Try to merge with last thinking part
	if len(b.current.Parts) > 0 {
		last := &b.current.Parts[len(b.current.Parts)-1]
		if last.IsThinking() {
			last.Text += thinking

			b.emit(ui.MessagePartUpdated{
				MessageID: b.current.ID,
				PartIndex: len(b.current.Parts) - 1,
				Part:      *last,
			})

			return
		}
	}

	// Add new thinking part
	p := part.Part{Type: part.Thinking, Text: thinking}

	b.current.Parts = append(b.current.Parts, p)

	b.emit(ui.MessagePartAdded{
		MessageID: b.current.ID,
		Part:      p,
	})
}

// AddToolCall adds a pending tool call to the current message with pre-formatted args.
func (b *Builder) AddToolCall(id, name string, args map[string]any) {
	if b.current == nil {
		debug.Log("[builder] AddToolCall called without active message")

		return
	}

	p := part.Part{
		Type: part.Tool,
		Call: &call.Call{
			ID:    id,
			Name:  name,
			State: call.Pending,
		},
	}

	p.Call.SetArgs(args)

	b.current.Parts = append(b.current.Parts, p)

	b.emit(ui.MessagePartAdded{
		MessageID: b.current.ID,
		Part:      p,
	})
}

// StartToolCall transitions a Pending tool call to Running and emits MessagePartUpdated.
func (b *Builder) StartToolCall(id string) {
	if b.current == nil {
		debug.Log("[builder] StartToolCall called without active message")

		return
	}

	for i := len(b.current.Parts) - 1; i >= 0; i-- {
		p := &b.current.Parts[i]
		if p.IsTool() && p.Call != nil &&
			p.Call.ID == id && p.Call.State == call.Pending {
			p.Call.MarkRunning()

			b.emit(ui.MessagePartUpdated{
				MessageID: b.current.ID,
				PartIndex: i,
				Part:      *p,
			})

			return
		}
	}
}

// CompleteToolCall marks a tool call as complete with its result, matching by ID.
func (b *Builder) CompleteToolCall(ctx context.Context, id, result string, err error) {
	if b.current == nil {
		debug.Log("[builder] CompleteToolCall called without active message")

		return
	}

	// Find the matching pending or running tool by ID
	for i := len(b.current.Parts) - 1; i >= 0; i-- {
		p := &b.current.Parts[i]
		if p.IsTool() && p.Call != nil &&
			p.Call.ID == id && (p.Call.State == call.Pending || p.Call.State == call.Running) {
			if err != nil {
				p.Call.Fail(err, b.Estimate(ctx, err.Error()))
			} else {
				p.Call.Complete(result, b.Estimate(ctx, result))
			}

			b.emit(ui.MessagePartUpdated{
				MessageID: b.current.ID,
				PartIndex: i,
				Part:      *p,
			})

			return
		}
	}
}

// FinalizeAssistant completes the current assistant message and emits MessageFinalized.
func (b *Builder) FinalizeAssistant() {
	if b.current == nil {
		debug.Log("[builder] FinalizeAssistant called without active message")

		return
	}

	b.emit(ui.MessageFinalized{Message: *b.current})

	b.current = nil
}

// SetError sets an error on the current assistant message.
func (b *Builder) SetError(err error) {
	if b.current == nil {
		debug.Log("[builder] SetError called without active message")

		return
	}

	b.current.Error = err
}

// AddAssistantMessage adds a complete assistant message to the API history.
// This should be called after receiving a response from the provider.
func (b *Builder) AddAssistantMessage(msg message.Message) {
	b.history.Add(message.Message{
		Role:              roles.Assistant,
		Content:           msg.Content,
		Thinking:          msg.Thinking,
		ThinkingSignature: msg.ThinkingSignature,
		Calls:             msg.Calls,
		Tokens:            msg.Tokens,
		CreatedAt:         time.Now(),
	})
}

// AddToolResult adds a tool execution result to the API history.
// When estimatedTokens > 0, it is used directly (avoids re-estimation).
func (b *Builder) AddToolResult(ctx context.Context, toolName, toolCallID, result string, estimatedTokens int) {
	tokensVal := estimatedTokens
	if tokensVal == 0 {
		tokensVal = b.Estimate(ctx, result)
	}

	b.history.Add(message.Message{
		Role:       roles.Tool,
		ToolName:   toolName,
		ToolCallID: toolCallID,
		Content:    result,
		Tokens:     message.Tokens{Total: tokensVal},
		CreatedAt:  time.Now(),
	})
}

// AddEphemeralToolResult adds a tool result that will be pruned after one model turn.
// Used for pre-execution errors (tool not found, policy deny, parse failures, etc.)
// that the model needs to see once but shouldn't persist in history.
// When estimatedTokens > 0, it is used directly (avoids re-estimation).
func (b *Builder) AddEphemeralToolResult(
	ctx context.Context,
	toolName, toolCallID, result string,
	estimatedTokens int,
) {
	tokensVal := estimatedTokens
	if tokensVal == 0 {
		tokensVal = b.Estimate(ctx, result)
	}

	b.history.Add(message.Message{
		Role:       roles.Tool,
		ToolName:   toolName,
		ToolCallID: toolCallID,
		Content:    result,
		Type:       message.Ephemeral,
		Tokens:     message.Tokens{Total: tokensVal},
		CreatedAt:  time.Now(),
	})
}

// AddDisplayMessage adds a display-only message to the history and emits MessageAdded.
// Display-only messages are visible in the UI and persisted but never sent to any LLM.
func (b *Builder) AddDisplayMessage(ctx context.Context, role roles.Role, content string) {
	msg := message.Message{
		ID:        b.NextID(),
		Role:      role,
		Content:   content,
		Type:      message.DisplayOnly,
		Tokens:    message.Tokens{Total: b.Estimate(ctx, content)},
		CreatedAt: time.Now(),
		Parts:     []part.Part{{Type: part.Content, Text: content}},
	}

	b.history.Add(message.Message{
		Role:      role,
		Content:   content,
		Type:      message.DisplayOnly,
		Tokens:    message.Tokens{Total: msg.Tokens.Total},
		CreatedAt: msg.CreatedAt,
	})

	b.emit(ui.MessageAdded{Message: msg})
}

// AddBookmark adds a structural bookmark divider to the history and emits MessageAdded.
// Bookmarks are rendered as separators in the UI, persisted, but never sent to any LLM.
func (b *Builder) AddBookmark(label string) {
	msg := message.Message{
		ID:        b.NextID(),
		Role:      roles.System,
		Content:   label,
		Type:      message.Bookmark,
		CreatedAt: time.Now(),
		Parts:     []part.Part{{Type: part.Content, Text: label}},
	}

	b.history.Add(message.Message{
		Role:      roles.System,
		Content:   label,
		Type:      message.Bookmark,
		CreatedAt: msg.CreatedAt,
	})

	b.emit(ui.MessageAdded{Message: msg})
}

// AddMetadata adds a structured metadata message to the history.
// Metadata is not rendered in the UI and never sent to any LLM, but persists in sessions
// and is included in structured exports (JSON/JSONL).
func (b *Builder) AddMetadata(data map[string]any) {
	encoded, err := json.Marshal(data)
	if err != nil {
		debug.Log("[builder] metadata marshal error: %v", err)

		return
	}

	b.history.Add(message.Message{
		Role:      roles.System,
		Content:   string(encoded),
		Type:      message.Metadata,
		CreatedAt: time.Now(),
	})
}

// Clear removes all messages except the system prompt from the history.
func (b *Builder) Clear() {
	b.history.Clear()

	b.current = nil
}

// DropN removes the last n messages from the history.
func (b *Builder) DropN(n int) {
	b.history.DropN(n)
}

// Len returns the number of messages in the history.
func (b *Builder) Len() int {
	return len(b.history)
}

// UpdateSystemPrompt updates the system prompt without clearing conversation history.
// This is used when switching modes — the conversation context is preserved.
// If the system prompt is missing (e.g., after clear on empty history), it is prepended.
func (b *Builder) UpdateSystemPrompt(ctx context.Context, prompt string) {
	if len(b.history) > 0 && b.history[0].Role == roles.System {
		b.history[0].Content = prompt
		b.history[0].Tokens.Total = b.Estimate(ctx, prompt)
	} else {
		b.history = append(message.New(message.Message{
			Role:    roles.System,
			Content: prompt,
			Tokens:  message.Tokens{Total: b.Estimate(ctx, prompt)},
		}), b.history...)
	}
}

// InjectMessage adds a synthetic message to the API history.
// The assistant emits a SyntheticInjected event for UI display separately.
func (b *Builder) InjectMessage(ctx context.Context, role roles.Role, content string) {
	content = strings.TrimRight(content, " \t\n\r")

	b.history.Add(message.Message{
		Role:      role,
		Content:   content,
		Type:      message.Synthetic,
		Tokens:    message.Tokens{Total: b.Estimate(ctx, content)},
		CreatedAt: time.Now(),
	})
}

// RebuildAfterCompaction replaces conversation history with a compaction summary
// and preserved recent message. The system prompt (index 0) is kept in place.
// summaryTokens is the pre-computed token estimate for the compaction summary.
func (b *Builder) RebuildAfterCompaction(summary string, summaryTokens int, preserved message.Messages) {
	// Keep system prompt, clear everything else
	if len(b.history) > 0 {
		systemPrompt := b.history[0]

		b.history = message.New(systemPrompt)
	}

	// Add compaction summary as a normal user message (not synthetic).
	// This must survive EjectSyntheticMessages — it's the model's primary
	// context for everything that happened before compaction.
	b.history.Add(message.Message{
		Role:      roles.User,
		Content:   summary,
		Tokens:    message.Tokens{Total: summaryTokens},
		CreatedAt: time.Now(),
	})

	// Re-add preserved messages
	for _, msg := range preserved {
		b.history.Add(msg)
	}

	b.current = nil
}

// UpdateThinking replaces the thinking content on the message at the given history index.
// Used by thinking management to mutate the builder in place (strip or rewrite).
func (b *Builder) UpdateThinking(index int, thinking string) {
	if index >= 0 && index < len(b.history) {
		b.history[index].Thinking = thinking
		b.history[index].ThinkingSignature = "" // Signature is invalidated when thinking text is mutated.
	}
}

// EjectSyntheticMessages removes all synthetic messages from history.
// Used when eject is enabled — synthetics influence one turn, then get stripped.
func (b *Builder) EjectSyntheticMessages() {
	b.history = b.history.WithoutSyntheticMessages()
}

// EjectEphemeralMessages removes ephemeral tool results and their matching calls from history.
// Ephemeral messages influence one turn, then get stripped along with their paired calls.
func (b *Builder) EjectEphemeralMessages() {
	b.history.EjectEphemeralMessages()
}

// PruneEmptyAssistantMessages removes assistant messages with no content, thinking, or tool calls.
func (b *Builder) PruneEmptyAssistantMessages() {
	b.history.PruneEmptyAssistantMessages()
}

// TrimDuplicateSynthetics removes duplicate synthetic messages from history,
// keeping only the most recent occurrence of each.
func (b *Builder) TrimDuplicateSynthetics() {
	b.history.TrimDuplicateSynthetics()
}

// DeduplicateSystemMessages keeps only the first system message, removing duplicates.
func (b *Builder) DeduplicateSystemMessages() {
	b.history.KeepFirstByRole(roles.System)
}

// Restore replaces the conversation history with the given message.
// The first message in msgs should be the system prompt.
func (b *Builder) Restore(msgs message.Messages) {
	b.history = msgs
	b.current = nil
}

// Turns returns conversation turns as sdk.Turn slices.
// Includes only normal user/assistant text messages — excludes system, tool,
// synthetic, and ephemeral message.
func (b *Builder) Turns() []sdk.Turn {
	var turns []sdk.Turn

	for _, m := range b.history {
		if m.IsSynthetic() || m.IsEphemeral() || m.IsInternal() {
			continue
		}

		if m.Role != roles.User && m.Role != roles.Assistant {
			continue
		}

		turns = append(turns, sdk.Turn{
			Role:    m.Role.String(),
			Content: m.Content,
		})
	}

	return turns
}

// SystemPrompt returns the assembled system prompt sent to the model.
func (b *Builder) SystemPrompt() string {
	if len(b.history) > 0 && b.history[0].Role == roles.System {
		return b.history[0].Content
	}

	return ""
}

// PruneToolResults prunes old tool results in the history in-place.
// Takes an explicit local estimate function (not the stored context-aware callback)
// because pruning is high-frequency and should always use local estimation.
func (b *Builder) PruneToolResults(protectTokens, argThreshold int, estimate func(string) int) {
	b.history.PruneToolResultsInPlace(protectTokens, argThreshold, estimate)
}

// BackfillToolTokens distributes an API-reported token delta across tool result
// messages added since the last assistant message. Walks backward from the end.
// Single tool result (common case) gets the full delta; multiple distribute
// proportionally by content length.
func (b *Builder) BackfillToolTokens(delta int) {
	if delta <= 0 {
		return
	}

	var toolIndices []int

	var totalLen int

	for i := len(b.history) - 1; i >= 0; i-- {
		msg := b.history[i]
		if msg.Role == roles.Assistant {
			break
		}

		if msg.Role == roles.Tool {
			toolIndices = append(toolIndices, i)

			totalLen += len(msg.Content)
		}
	}

	if len(toolIndices) == 0 || totalLen == 0 {
		return
	}

	if len(toolIndices) == 1 {
		b.history[toolIndices[0]].Tokens.Total = delta

		return
	}

	var assigned int

	for j, idx := range toolIndices {
		if j == len(toolIndices)-1 {
			b.history[idx].Tokens.Total = delta - assigned
		} else {
			share := delta * len(b.history[idx].Content) / totalLen

			b.history[idx].Tokens.Total = share

			assigned += share
		}
	}
}
