package slash

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/shlex"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/ui"
)

// ErrUsage is returned by commands to signal that usage help should be shown.
var ErrUsage = errors.New("invalid argument")

// categoryOrder defines the deterministic display order for command categories.
// Categories not in this list sort alphabetically after the known ones.
var categoryOrder = []string{
	"agent", "session", "tools", "context",
	"execution", "config", "todo", "search", "system",
}

// Command defines a slash command.
type Command struct {
	Name        string
	Aliases     []string
	Description string
	Hints       string // Argument hint text shown as ghost text in TUI (e.g. "[name]", "<file> [options]").
	Category    string // Grouping label for /help display (e.g. "agent", "session", "tools").
	Forward     bool   // Forward causes the result to be sent to the LLM as a user message instead of displayed as a CommandResult.
	Silent      bool   // Silent suppresses the pre-execution command echo in chat history.
	Execute     func(ctx context.Context, c Context, args ...string) (string, error)
}

// Usage returns the full usage string (e.g. "/mode [name]").
func (c Command) Usage() string {
	if c.Hints != "" {
		return c.Name + " " + c.Hints
	}

	return c.Name
}

// Registry holds all registered slash commands.
type Registry struct {
	commands map[string]Command
	aliases  map[string]string // alias -> canonical name
}

// New creates a new slash command registry with built-in /help.
func New(cmd ...Command) *Registry {
	r := &Registry{
		commands: make(map[string]Command),
		aliases:  make(map[string]string),
	}

	r.Register(Command{
		Name:        "/help",
		Hints:       "[command]",
		Description: "Show available commands",
		Category:    "system",
		Execute: func(_ context.Context, c Context, args ...string) (string, error) {
			if len(args) > 0 {
				name := strings.ToLower(strings.Join(args, " "))
				if !strings.HasPrefix(name, "/") {
					name = "/" + name
				}

				if canonical, ok := r.aliases[name]; ok {
					name = canonical
				}

				cmd, exists := r.commands[name]
				if !exists {
					return "", fmt.Errorf("unknown command: %s", name)
				}

				var lines []string

				if cmd.Category != "" {
					lines = append(
						lines,
						"**Category:** "+strings.Title(cmd.Category),
					)
				}

				lines = append(lines, fmt.Sprintf("**%s** — %s", cmd.Name, cmd.Description))

				lines = append(lines, fmt.Sprintf("Usage: `%s`", cmd.Usage()))
				if len(cmd.Aliases) > 0 {
					lines = append(lines, "Aliases: "+strings.Join(cmd.Aliases, ", "))
				}

				return strings.Join(lines, "\n"), nil
			}

			var items []ui.PickerItem

			for _, cat := range r.Categories() {
				group := strings.Title(cat) //nolint:staticcheck
				for _, cmd := range r.ByCategory(cat) {
					label := cmd.Usage()
					if len(cmd.Aliases) > 0 {
						label += " (" + strings.Join(cmd.Aliases, ", ") + ")"
					}

					items = append(items, ui.PickerItem{
						Group:       group,
						Label:       label,
						Description: cmd.Description,
						Action:      ui.RunCommand{Name: cmd.Name},
					})
				}
			}

			c.EventChan() <- ui.PickerOpen{
				Title: "Commands:",
				Items: items,
			}

			return "", nil
		},
	})

	r.Register(cmd...)

	return r
}

// Register adds a command to the registry. Names and aliases are stored lowercase for case-insensitive lookup.
func (r *Registry) Register(cmd ...Command) {
	for _, c := range cmd {
		key := strings.ToLower(c.Name)

		r.commands[key] = c

		for _, alias := range c.Aliases {
			r.aliases[strings.ToLower(alias)] = key
		}
	}
}

// Handle processes user input. Returns (message, handled, forward, error).
// If input doesn't start with "/", returns ("", false, nil).
// When forward is true, the message should be sent to the LLM as a user message.
//
// Handle is the single source of command echoes in the TUI.
// Non-Silent commands emit CommandResult{Command: input} before execution.
// All other CommandResult emissions should use Message/Error only.
func (r *Registry) Handle(ctx context.Context, sctx Context, input string) (string, bool, bool, error) {
	if !strings.HasPrefix(input, "/") {
		return "", false, false, nil
	}

	parts, err := shlex.Split(input)
	if err != nil {
		parts = strings.Fields(input)
	}

	if len(parts) == 0 {
		return "", false, false, nil
	}

	name := strings.ToLower(parts[0])
	args := parts[1:]

	// Check for alias
	if canonical, ok := r.aliases[name]; ok {
		name = canonical
	}

	cmd, exists := r.commands[name]
	if !exists {
		debug.Log("[slash] unknown command: %s", name)

		return "", true, false, fmt.Errorf("unknown command: %s (try /help)", name)
	}

	debug.Log("[slash] %s (args=%d)", name, len(args))

	// Show command in chat history before execution so events from the
	// handler (spinners, synthetic injections) appear AFTER the command text.
	if !cmd.Silent {
		sctx.EventChan() <- ui.CommandResult{Command: input}
	}

	msg, err := cmd.Execute(ctx, sctx, args...)
	if errors.Is(err, ErrUsage) {
		if err == ErrUsage { //nolint:errorlint,goerr113 // exact match = bare ErrUsage, no context
			return "", true, false, fmt.Errorf("usage: %s", cmd.Usage())
		}

		return "", true, false, fmt.Errorf("%w\nusage: %s", err, cmd.Usage())
	}

	return msg, true, cmd.Forward, err
}

// Lookup checks if a command name is already registered (case-insensitive).
func (r *Registry) Lookup(name string) (Command, bool) {
	cmd, exists := r.commands[strings.ToLower(name)]

	return cmd, exists
}

// HintFor returns the hint text for a command name, resolving aliases.
// Returns empty string if the command is not found or has no hints.
func (r *Registry) HintFor(name string) string {
	name = strings.ToLower(name)

	if canonical, ok := r.aliases[name]; ok {
		name = canonical
	}

	cmd, exists := r.commands[name]
	if !exists {
		return ""
	}

	return cmd.Hints
}

// Sorted returns all registered commands sorted by name.
func (r *Registry) Sorted() []Command {
	cmds := make([]Command, 0, len(r.commands))
	for _, cmd := range r.commands {
		cmds = append(cmds, cmd)
	}

	slices.SortFunc(cmds, func(a, b Command) int {
		return cmp.Compare(a.Name, b.Name)
	})

	return cmds
}

// Categories returns the ordered list of categories present in the registry.
// Known categories appear in categoryOrder; unknown ones sort alphabetically after.
func (r *Registry) Categories() []string {
	present := make(map[string]bool)

	for _, cmd := range r.commands {
		if cmd.Category != "" {
			present[cmd.Category] = true
		}
	}

	var ordered []string

	for _, cat := range categoryOrder {
		if present[cat] {
			ordered = append(ordered, cat)
			delete(present, cat)
		}
	}

	// Remaining (unknown) categories, sorted alphabetically.
	var extra []string

	for cat := range present {
		extra = append(extra, cat)
	}

	slices.Sort(extra)

	return append(ordered, extra...)
}

// ByCategory returns commands in the given category, sorted alphabetically by name.
func (r *Registry) ByCategory(cat string) []Command {
	var cmds []Command

	for _, cmd := range r.commands {
		if cmd.Category == cat {
			cmds = append(cmds, cmd)
		}
	}

	slices.SortFunc(cmds, func(a, b Command) int {
		return cmp.Compare(a.Name, b.Name)
	})

	return cmds
}
