package ui

// Action represents a UI-triggered action (keybinding, etc).
type Action interface{ isAction() }

// ToggleVerbose toggles visibility of thinking content.
type ToggleVerbose struct{}

func (ToggleVerbose) isAction() {}

// NextAgent cycles to the next available agent.
type NextAgent struct{}

func (NextAgent) isAction() {}

// NextMode cycles to the next available mode.
type NextMode struct{}

func (NextMode) isAction() {}

// ToggleThink toggles thinking off ↔ on (true).
type ToggleThink struct{}

func (ToggleThink) isAction() {}

// CycleThink cycles through: off → on → low → medium → high → off.
type CycleThink struct{}

func (CycleThink) isAction() {}

// ToggleAuto toggles the Auto mode on/off.
type ToggleAuto struct{}

func (ToggleAuto) isAction() {}

// ToggleSandbox toggles sandbox enforcement on/off.
type ToggleSandbox struct{}

func (ToggleSandbox) isAction() {}

// SelectModel switches to a specific provider/model.
type SelectModel struct {
	Provider string
	Model    string
}

func (SelectModel) isAction() {}

// RunCommand executes a slash command by name (e.g. "/info", "/model").
type RunCommand struct {
	Name string
}

func (RunCommand) isAction() {}

// ResumeSession resumes a saved session by ID.
type ResumeSession struct {
	SessionID string
}

func (ResumeSession) isAction() {}

// TodoEdited carries the raw text from $EDITOR back to the assistant for parsing.
type TodoEdited struct {
	Text string
}

func (TodoEdited) isAction() {}

// UndoSnapshot is dispatched when the user selects a snapshot from the /undo picker.
// Triggers the second picker asking what to rewind.
type UndoSnapshot struct {
	Hash         string
	MessageIndex int
}

func (UndoSnapshot) isAction() {}

// UndoExecute is dispatched when the user selects a rewind mode (code/messages/both).
type UndoExecute struct {
	Hash         string
	MessageIndex int
	Mode         string // "code", "messages", or "both"
}

func (UndoExecute) isAction() {}

// ConfirmOnce signals the user approved a single command execution.
type ConfirmOnce struct{}

func (ConfirmOnce) isAction() {}

// ConfirmSession signals the user approved all matching commands for this session.
type ConfirmSession struct{}

func (ConfirmSession) isAction() {}

// ConfirmPatternProject signals the user approved all matching commands (project scope).
type ConfirmPatternProject struct{}

func (ConfirmPatternProject) isAction() {}

// ConfirmPatternGlobal signals the user approved all matching commands (global scope).
type ConfirmPatternGlobal struct{}

func (ConfirmPatternGlobal) isAction() {}

// ConfirmReject signals the user denied the command.
type ConfirmReject struct{}

func (ConfirmReject) isAction() {}
