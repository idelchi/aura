package todo

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// ErrInvalidIndex is returned when an operation targets an out-of-range index.
var ErrInvalidIndex = errors.New("invalid todo index")

// Status represents the current state of a todo item.
type Status string

const (
	Pending    Status = "pending"
	InProgress Status = "in_progress"
	Completed  Status = "completed"
)

// Todo represents a single task with its content and current state.
type Todo struct {
	Content string `json:"content"`
	Status  Status `json:"status"`
}

// List holds an ordered collection of todos and an optional summary.
type List struct {
	Summary string `json:"summary,omitempty"`
	Items   []Todo `json:"items"`
}

// New creates an empty List.
func New() *List {
	return &List{
		Items: []Todo{},
	}
}

// Replace replaces the entire todo list with the provided todos.
func (l *List) Replace(todos []Todo) {
	l.Items = todos
}

// Get returns the current todo list.
func (l *List) Get() []Todo {
	if l == nil {
		return nil
	}

	return l.Items
}

// Len returns the total number of items in the list.
func (l *List) Len() int {
	if l == nil {
		return 0
	}

	return len(l.Items)
}

// IsEmpty reports whether the list has no items and no summary.
func (l *List) IsEmpty() bool {
	if l == nil {
		return true
	}

	return len(l.Items) == 0 && l.Summary == ""
}

// SetSummary updates the executive summary.
func (l *List) SetSummary(summary string) {
	l.Summary = summary
}

// Add appends a new todo to the list.
func (l *List) Add(todo Todo) {
	l.Items = append(l.Items, todo)
}

// MarkStatus updates the status of the todo at the given index.
func (l *List) MarkStatus(index int, status Status) error {
	if index < 0 || index >= len(l.Items) {
		return ErrInvalidIndex
	}

	l.Items[index].Status = status

	return nil
}

// Remove removes the todo at the given index.
func (l *List) Remove(index int) error {
	if index < 0 || index >= len(l.Items) {
		return ErrInvalidIndex
	}

	l.Items = append(l.Items[:index], l.Items[index+1:]...)

	return nil
}

// Complete marks the todo at the given index as completed.
func (l *List) Complete(index int) error {
	return l.MarkStatus(index, Completed)
}

// AllCompleted reports whether all items are completed, or the list is empty.
func (l *List) AllCompleted() bool {
	if l == nil || len(l.Items) == 0 {
		return true
	}

	return !slices.ContainsFunc(l.Items, func(t Todo) bool {
		return t.Status != Completed
	})
}

// Counts returns the number of pending, in-progress, and completed items.
func (l *List) Counts() (pending, inProgress, completed int) {
	if l == nil {
		return 0, 0, 0
	}

	for _, item := range l.Items {
		switch item.Status {
		case Pending:
			pending++
		case InProgress:
			inProgress++
		case Completed:
			completed++
		}
	}

	return pending, inProgress, completed
}

// FindInProgress returns the indices of all in-progress todos.
func (l *List) FindInProgress() []int {
	return l.ByStatus(InProgress)
}

// FindPending returns the indices of all pending todos.
func (l *List) FindPending() []int {
	return l.ByStatus(Pending)
}

// ByStatus returns the indices of all todos matching the given status.
func (l *List) ByStatus(status Status) []int {
	if l == nil {
		return nil
	}

	var indices []int

	for i, todo := range l.Items {
		if todo.Status == status {
			indices = append(indices, i)
		}
	}

	return indices
}

// validStatuses maps status strings to their typed values.
var validStatuses = map[string]Status{
	string(Pending):    Pending,
	string(InProgress): InProgress,
	string(Completed):  Completed,
}

// todoLineRe matches lines like "1. [pending] Some task content".
var todoLineRe = regexp.MustCompile(`^\d+\.\s+\[(\w+)\]\s+(.+)$`)

// summaryPrefix is the markdown prefix for the summary line.
const summaryPrefix = "**Summary:** "

// Parse parses a text representation of a todo list (as produced by String)
// back into a List. Unrecognized lines are silently ignored. Returns an error
// only if a matched todo line contains an invalid status.
func Parse(text string) (*List, error) {
	list := New()

	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(line)

		if after, ok := strings.CutPrefix(line, summaryPrefix); ok {
			list.Summary = after

			continue
		}

		m := todoLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		status, ok := validStatuses[m[1]]
		if !ok {
			return nil, fmt.Errorf("invalid status %q on line: %s", m[1], line)
		}

		list.Items = append(list.Items, Todo{Content: m[2], Status: status})
	}

	return list, nil
}

// String formats the todo list for display.
func (l *List) String() string {
	if l == nil {
		return ""
	}

	var b strings.Builder

	if l.Summary != "" {
		fmt.Fprintf(&b, "**Summary:** %s\n\n", l.Summary)
	}

	for i, todo := range l.Items {
		fmt.Fprintf(&b, "%d. [%s] %s\n", i+1, todo.Status, todo.Content)
	}

	return b.String()
}
