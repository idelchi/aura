package ui

import (
	"context"
	"fmt"
	"strings"

	humanize "github.com/dustin/go-humanize"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/part"
	"github.com/idelchi/aura/pkg/llm/thinking"
)

// EventTag categorizes events into semantic groups.
type EventTag int

const (
	TagMessage EventTag = iota // Message lifecycle (Added, Started, PartAdded, PartUpdated, Finalized)
	TagStatus                  // System state (StatusChanged, AssistantDone, WaitingForInput, UserMessagesProcessed, SpinnerMessage, SyntheticInjected)
	TagTool                    // Tool execution (ToolOutputDelta, ToolConfirmRequired)
	TagDialog                  // User interaction (AskRequired, TodoEditRequested)
	TagSession                 // Session/command (SessionRestored, SlashCommandHandled, CommandResult)
	TagControl                 // UI control (PickerOpen, Flush)
)

// String returns the tag name.
func (t EventTag) String() string {
	switch t {
	case TagMessage:
		return "message"
	case TagStatus:
		return "status"
	case TagTool:
		return "tool"
	case TagDialog:
		return "dialog"
	case TagSession:
		return "session"
	case TagControl:
		return "control"
	default:
		return "unknown"
	}
}

// Event is a marker interface for all events sent through the UI event channel.
type Event interface {
	isEvent()
	Tag() EventTag
}

// StatusChanged signals the status bar should update.
type StatusChanged struct {
	Status Status
}

func (StatusChanged) isEvent()      {}
func (StatusChanged) Tag() EventTag { return TagStatus }

// Status holds the current assistant status for display.
type Status struct {
	Agent    string
	Mode     string
	Provider string
	Model    string
	Think    thinking.Value
	Tokens   struct {
		Used    int
		Max     int
		Percent float64
	}
	Sandbox struct {
		Enabled   bool
		Requested bool
	}
	Snapshots bool // snapshot tracking enabled
	Steps     struct {
		Current int // iteration within the turn (0 = idle)
		Max     int // configured max steps
	}
}

// DisplayHints holds UI-specific display preferences.
type DisplayHints struct {
	Verbose bool
	Auto    bool
}

// DisplayHintsChanged is emitted when UI display preferences change.
type DisplayHintsChanged struct {
	Hints DisplayHints
}

func (DisplayHintsChanged) isEvent()      {}
func (DisplayHintsChanged) Tag() EventTag { return TagStatus }

// compactSI formats n with SI suffix and the given decimal digits, stripping the space
// that go-humanize inserts (e.g. 131072, 0 → "131k").
func compactSI(n, digits int) string {
	return strings.ReplaceAll(humanize.SIWithDigits(float64(n), digits, ""), " ", "")
}

// TokensDisplay returns a formatted token usage string (e.g. "tokens: 12.4k/131k (10%)").
func (s Status) TokensDisplay() string {
	if s.Tokens.Max == 0 {
		return ""
	}

	return fmt.Sprintf(
		"tokens: %s/%s (%.0f%%)",
		compactSI(s.Tokens.Used, 1),
		compactSI(s.Tokens.Max, 0),
		s.Tokens.Percent,
	)
}

// StatusLine returns the full status bar string with all parts joined by " • ".
func (s Status) StatusLine(hints DisplayHints) string {
	parts := []string{s.Agent, s.Mode, s.Model}

	parts = append(parts, "think: "+s.Think.AsString())
	parts = append(parts, s.Provider)

	if s.Steps.Current > 0 {
		parts = append(parts, fmt.Sprintf("step %d/%d", s.Steps.Current, s.Steps.Max))
	}

	if td := s.TokensDisplay(); td != "" {
		parts = append(parts, td)
	}

	if s.Sandbox.Enabled {
		parts = append(parts, "🔒")
	} else {
		parts = append(parts, "🔓")
	}

	if s.Snapshots {
		parts = append(parts, "📸")
	}

	if hints.Verbose {
		parts = append(parts, "verbose")
	}

	if hints.Auto {
		parts = append(parts, "auto")
	}

	// Filter empty parts (e.g. Mode when agent has no default mode)
	filtered := parts[:0]
	for _, p := range parts {
		if p != "" {
			filtered = append(filtered, p)
		}
	}

	return strings.Join(filtered, " • ")
}

// Prompt returns a formatted prompt string for the simple UI.
func (s Status) Prompt() string {
	if s.Tokens.Max > 0 && s.Tokens.Percent > 0 {
		return fmt.Sprintf("[%.1f%%] > ", s.Tokens.Percent)
	}

	return "> "
}

// WelcomeInfo returns a multi-line welcome block with arrow bullets.
func (s Status) WelcomeInfo() string {
	line := "→ " + s.Agent
	if s.Model != "" {
		line += " (" + s.Model + ")"
	}

	lines := []string{line}

	lines = append(lines, "→ Provider: "+s.Provider)
	lines = append(lines, "→ Type /help for available commands")

	return strings.Join(lines, "\n")
}

// AssistantPrompt returns a rich role label for assistant output (e.g. "Aura (model, 🧠 high, edit): ").
func (s Status) AssistantPrompt() string {
	var parts []string

	if s.Model != "" {
		parts = append(parts, s.Model)
	}

	if s.Think.AsString() != "false" {
		parts = append(parts, "🧠 "+s.Think.AsString())
	}

	if s.Mode != "" {
		parts = append(parts, s.Mode)
	}

	if len(parts) == 0 {
		return "Aura: "
	}

	return fmt.Sprintf("Aura (%s): ", strings.Join(parts, ", "))
}

// UserPrompt returns a rich role label for user messages (e.g. "You (model, edit): ").
func (s Status) UserPrompt() string {
	var parts []string

	if s.Model != "" {
		parts = append(parts, s.Model)
	}

	if s.Mode != "" {
		parts = append(parts, s.Mode)
	}

	if len(parts) == 0 {
		return "You: "
	}

	return fmt.Sprintf("You (%s): ", strings.Join(parts, ", "))
}

// ContextDisplay returns context stats like "Context: 12.4k / 131k tokens (10%), 42 messages".
func (s Status) ContextDisplay(msgCount int) string {
	return fmt.Sprintf("Context: %s / %s tokens (%.0f%%), %d messages",
		compactSI(s.Tokens.Used, 1), compactSI(s.Tokens.Max, 0), s.Tokens.Percent, msgCount)
}

// WindowDisplay returns the context window size like "Context window: 131k tokens".
func (s Status) WindowDisplay() string {
	return fmt.Sprintf("Context window: %s tokens", compactSI(s.Tokens.Max, 0))
}

// Level indicates the severity of a CommandResult for UI styling.
type Level int

const (
	LevelInfo    Level = iota // Default — gray, auto-clears after 5s
	LevelSuccess              // Green, auto-clears after 5s
	LevelWarn                 // Yellow, auto-clears after 5s
	LevelError                // Red, persists until next input
)

// CommandResult signals the result of a slash command.
type CommandResult struct {
	Command string
	Message string
	Error   error
	Level   Level // Styling hint (zero value = LevelInfo)
	Clear   bool  // when true, UI clears all displayed messages first
	Inline  bool  // when true, multi-line messages print inline instead of opening a pager
}

func (CommandResult) isEvent()      {}
func (CommandResult) Tag() EventTag { return TagSession }

// AssistantDone signals the assistant has finished processing.
type AssistantDone struct {
	Error     error
	Cancelled bool // True if cancelled via ESC
}

func (AssistantDone) isEvent()      {}
func (AssistantDone) Tag() EventTag { return TagStatus }

// WaitingForInput signals the UI should accept user input.
type WaitingForInput struct{}

func (WaitingForInput) isEvent()      {}
func (WaitingForInput) Tag() EventTag { return TagStatus }

// UserMessagesProcessed signals the backend has started processing user messages.
type UserMessagesProcessed struct {
	Texts []string
}

func (UserMessagesProcessed) isEvent()      {}
func (UserMessagesProcessed) Tag() EventTag { return TagStatus }

// SessionRestored carries restored conversation messages for UI display after resume.
type SessionRestored struct {
	Messages message.Messages
}

func (SessionRestored) isEvent()      {}
func (SessionRestored) Tag() EventTag { return TagSession }

// SlashCommandHandled signals a slash command was processed (no streaming follows).
type SlashCommandHandled struct {
	Text string
}

func (SlashCommandHandled) isEvent()      {}
func (SlashCommandHandled) Tag() EventTag { return TagSession }

// SpinnerMessage updates the spinner text during streaming.
type SpinnerMessage struct {
	Text string
}

func (SpinnerMessage) isEvent()      {}
func (SpinnerMessage) Tag() EventTag { return TagStatus }

// ToolOutputDelta carries a single line of incremental tool output during execution.
type ToolOutputDelta struct {
	ToolName string
	Line     string
}

func (ToolOutputDelta) isEvent()      {}
func (ToolOutputDelta) Tag() EventTag { return TagTool }

// SyntheticInjected signals that a synthetic message was injected into the conversation.
type SyntheticInjected struct {
	Header  string // e.g. "[SYNTHETIC user]: loop_detection"
	Content string // Truncated message content for display
	Role    string // "user", "assistant", "system"
}

func (SyntheticInjected) isEvent()      {}
func (SyntheticInjected) Tag() EventTag { return TagStatus }

// Flush signals the UI to process all pending events before continuing.
type Flush struct {
	Done chan struct{}
}

func (Flush) isEvent()      {}
func (Flush) Tag() EventTag { return TagControl }

// AskOption represents a selectable choice in an ask prompt.
type AskOption struct {
	Label       string
	Description string
}

// AskRequired signals the LLM needs user input before proceeding.
type AskRequired struct {
	Question    string
	Options     []AskOption
	MultiSelect bool
	Response    chan<- string
}

func (AskRequired) isEvent()      {}
func (AskRequired) Tag() EventTag { return TagDialog }

// MessageAdded signals a complete message was added.
type MessageAdded struct {
	Message message.Message
}

func (MessageAdded) isEvent()      {}
func (MessageAdded) Tag() EventTag { return TagMessage }

// MessageStarted signals a new assistant message is beginning.
type MessageStarted struct {
	MessageID string
}

func (MessageStarted) isEvent()      {}
func (MessageStarted) Tag() EventTag { return TagMessage }

// MessagePartAdded signals a new part was added to the current message.
type MessagePartAdded struct {
	MessageID string
	Part      part.Part
}

func (MessagePartAdded) isEvent()      {}
func (MessagePartAdded) Tag() EventTag { return TagMessage }

// MessagePartUpdated signals an existing part was updated.
type MessagePartUpdated struct {
	MessageID string
	PartIndex int
	Part      part.Part
}

func (MessagePartUpdated) isEvent()      {}
func (MessagePartUpdated) Tag() EventTag { return TagMessage }

// MessageFinalized signals the current message is complete.
type MessageFinalized struct {
	Message message.Message
}

func (MessageFinalized) isEvent()      {}
func (MessageFinalized) Tag() EventTag { return TagMessage }

// UserInput contains text submitted by the user.
type UserInput struct {
	Text string
}

// UserAction represents a UI-triggered action (not from text input).
type UserAction struct {
	Action Action
}

// UI defines the interface for user interaction.
type UI interface {
	Run(ctx context.Context) error
	Events() chan<- Event
	Input() <-chan UserInput
	Actions() <-chan UserAction
	Cancel() <-chan struct{}
	SetHintFunc(func(string) string)
	SetWorkdir(string)
}

// PickerItem represents a single selectable item in a picker overlay.
type PickerItem struct {
	Group       string // Section header (provider name, category, etc.)
	Label       string // Main display text (model name, command usage, etc.)
	Description string // Secondary text rendered faint (optional)
	Icons       string // Capability markers (optional)
	Current     bool   // Highlight as currently active
	Disabled    bool   // Unreachable/non-selectable placeholder
	Action      Action // What to dispatch on selection
}

// TodoEditRequested signals the UI should open $EDITOR for todo list editing.
type TodoEditRequested struct {
	Content string // Serialized todo list text
}

func (TodoEditRequested) isEvent()      {}
func (TodoEditRequested) Tag() EventTag { return TagDialog }

// ConfirmAction is the user's response to a tool confirmation prompt.
type ConfirmAction int

const (
	// ConfirmAllow approves the command once.
	ConfirmAllow ConfirmAction = iota
	// ConfirmAllowSession approves all matching commands for this session (in-memory only).
	ConfirmAllowSession
	// ConfirmAllowPatternProject persists the pattern to project-scoped approval rules.
	ConfirmAllowPatternProject
	// ConfirmAllowPatternGlobal persists the pattern to global-scoped approval rules.
	ConfirmAllowPatternGlobal
	// ConfirmDeny blocks the command.
	ConfirmDeny
)

// ToolConfirmRequired signals that a tool call needs user approval before execution.
type ToolConfirmRequired struct {
	ToolName    string               // Tool name (e.g. "Bash", "Write", "mcp__server__tool")
	Description string               // What the tool does (from tool.Description())
	Detail      string               // Context for display: command for Bash, file path for Write, etc.
	DiffPreview string               // Unified diff preview (empty for non-file tools)
	Pattern     string               // Derived persistence pattern (e.g. "Bash:git commit*", "Write")
	Response    chan<- ConfirmAction // User's decision
}

func (ToolConfirmRequired) isEvent()      {}
func (ToolConfirmRequired) Tag() EventTag { return TagTool }

// PickerOpen signals the TUI should open a picker overlay.
type PickerOpen struct {
	Title string // Picker title (e.g. "Select model:", "Commands:")
	Items []PickerItem
}

func (PickerOpen) isEvent()      {}
func (PickerOpen) Tag() EventTag { return TagControl }

// Display returns a formatted text representation of the picker for non-TUI output.
func (p PickerOpen) Display() string {
	var b strings.Builder

	b.WriteString(p.Title)
	b.WriteByte('\n')

	lastGroup := ""

	for _, item := range p.Items {
		// Group header
		if item.Group != "" && item.Group != lastGroup {
			lastGroup = item.Group

			fmt.Fprintf(&b, "\n  %s\n", item.Group)
		}

		// Skip disabled placeholders
		if item.Disabled {
			continue
		}

		// Item line
		b.WriteString("    ")
		b.WriteString(item.Label)

		if item.Icons != "" {
			b.WriteString(item.Icons)
		}

		if item.Current {
			b.WriteString(" *")
		}

		if item.Description != "" {
			fmt.Fprintf(&b, "  %s", item.Description)
		}

		b.WriteByte('\n')
	}

	return b.String()
}
