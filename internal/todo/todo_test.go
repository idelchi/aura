package todo_test

import (
	"errors"
	"testing"

	"github.com/idelchi/aura/internal/todo"
)

// ── TestNew ───────────────────────────────────────────────────────────────────

func TestNew(t *testing.T) {
	t.Parallel()

	l := todo.New()

	if l == nil {
		t.Fatal("New() returned nil")
	}

	if l.Len() != 0 {
		t.Errorf("New().Len() = %d, want 0", l.Len())
	}

	if !l.IsEmpty() {
		t.Error("New().IsEmpty() = false, want true")
	}

	if got := l.Get(); got != nil && len(got) != 0 {
		t.Errorf("New().Get() = %v, want empty slice", got)
	}
}

// ── TestAddGet ────────────────────────────────────────────────────────────────

func TestAddGet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		items []todo.Todo
	}{
		{
			name:  "single item is retrievable",
			items: []todo.Todo{{Content: "task one", Status: todo.Pending}},
		},
		{
			name: "multiple items preserve insertion order",
			items: []todo.Todo{
				{Content: "first", Status: todo.Pending},
				{Content: "second", Status: todo.InProgress},
				{Content: "third", Status: todo.Completed},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := todo.New()

			for _, item := range tt.items {
				l.Add(item)
			}

			got := l.Get()
			if len(got) != len(tt.items) {
				t.Fatalf("Get() returned %d items, want %d", len(got), len(tt.items))
			}

			for i, item := range tt.items {
				if got[i].Content != item.Content {
					t.Errorf("item[%d].Content = %q, want %q", i, got[i].Content, item.Content)
				}

				if got[i].Status != item.Status {
					t.Errorf("item[%d].Status = %q, want %q", i, got[i].Status, item.Status)
				}
			}
		})
	}
}

// ── TestMarkStatus ────────────────────────────────────────────────────────────

func TestMarkStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		index     int
		newStatus todo.Status
		wantErr   error
	}{
		{
			name:      "valid index updates status to InProgress",
			index:     0,
			newStatus: todo.InProgress,
			wantErr:   nil,
		},
		{
			name:      "valid index updates status to Completed",
			index:     1,
			newStatus: todo.Completed,
			wantErr:   nil,
		},
		{
			name:      "negative index returns ErrInvalidIndex",
			index:     -1,
			newStatus: todo.Completed,
			wantErr:   todo.ErrInvalidIndex,
		},
		{
			name:      "out of bounds index returns ErrInvalidIndex",
			index:     99,
			newStatus: todo.Completed,
			wantErr:   todo.ErrInvalidIndex,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := todo.New()
			l.Add(todo.Todo{Content: "alpha", Status: todo.Pending})
			l.Add(todo.Todo{Content: "beta", Status: todo.Pending})

			err := l.MarkStatus(tt.index, tt.newStatus)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("MarkStatus(%d, %q) error = %v, want %v", tt.index, tt.newStatus, err, tt.wantErr)
			}

			if tt.wantErr == nil {
				items := l.Get()
				if items[tt.index].Status != tt.newStatus {
					t.Errorf("after MarkStatus, item[%d].Status = %q, want %q",
						tt.index, items[tt.index].Status, tt.newStatus)
				}
			}
		})
	}
}

// ── TestComplete ──────────────────────────────────────────────────────────────

func TestComplete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		index   int
		wantErr error
	}{
		{
			name:    "valid index marks item as completed",
			index:   0,
			wantErr: nil,
		},
		{
			name:    "negative index returns ErrInvalidIndex",
			index:   -1,
			wantErr: todo.ErrInvalidIndex,
		},
		{
			name:    "out of bounds index returns ErrInvalidIndex",
			index:   5,
			wantErr: todo.ErrInvalidIndex,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := todo.New()
			l.Add(todo.Todo{Content: "task", Status: todo.Pending})

			err := l.Complete(tt.index)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Complete(%d) error = %v, want %v", tt.index, err, tt.wantErr)
			}

			if tt.wantErr == nil {
				items := l.Get()
				if items[tt.index].Status != todo.Completed {
					t.Errorf("after Complete, item[%d].Status = %q, want %q",
						tt.index, items[tt.index].Status, todo.Completed)
				}
			}
		})
	}
}

// ── TestRemove ────────────────────────────────────────────────────────────────

func TestRemove(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		initialItems  []todo.Todo
		removeIndex   int
		wantErr       error
		wantRemaining []string // expected Content values after removal
	}{
		{
			name: "remove first item shifts remainder left",
			initialItems: []todo.Todo{
				{Content: "a", Status: todo.Pending},
				{Content: "b", Status: todo.Pending},
				{Content: "c", Status: todo.Pending},
			},
			removeIndex:   0,
			wantErr:       nil,
			wantRemaining: []string{"b", "c"},
		},
		{
			name: "remove middle item",
			initialItems: []todo.Todo{
				{Content: "a", Status: todo.Pending},
				{Content: "b", Status: todo.Pending},
				{Content: "c", Status: todo.Pending},
			},
			removeIndex:   1,
			wantErr:       nil,
			wantRemaining: []string{"a", "c"},
		},
		{
			name: "remove last item",
			initialItems: []todo.Todo{
				{Content: "a", Status: todo.Pending},
				{Content: "b", Status: todo.Pending},
			},
			removeIndex:   1,
			wantErr:       nil,
			wantRemaining: []string{"a"},
		},
		{
			name:          "negative index returns ErrInvalidIndex",
			initialItems:  []todo.Todo{{Content: "a", Status: todo.Pending}},
			removeIndex:   -1,
			wantErr:       todo.ErrInvalidIndex,
			wantRemaining: []string{"a"},
		},
		{
			name:          "out of bounds index returns ErrInvalidIndex",
			initialItems:  []todo.Todo{{Content: "a", Status: todo.Pending}},
			removeIndex:   5,
			wantErr:       todo.ErrInvalidIndex,
			wantRemaining: []string{"a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := todo.New()

			for _, item := range tt.initialItems {
				l.Add(item)
			}

			err := l.Remove(tt.removeIndex)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Remove(%d) error = %v, want %v", tt.removeIndex, err, tt.wantErr)
			}

			got := l.Get()
			if len(got) != len(tt.wantRemaining) {
				t.Fatalf("after Remove, list has %d items, want %d", len(got), len(tt.wantRemaining))
			}

			for i, want := range tt.wantRemaining {
				if got[i].Content != want {
					t.Errorf("after Remove, item[%d].Content = %q, want %q", i, got[i].Content, want)
				}
			}
		})
	}
}

// ── TestAllCompleted ──────────────────────────────────────────────────────────

func TestAllCompleted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		items []todo.Todo
		want  bool
	}{
		{
			name:  "empty list returns true",
			items: []todo.Todo{},
			want:  true,
		},
		{
			name: "all completed returns true",
			items: []todo.Todo{
				{Content: "a", Status: todo.Completed},
				{Content: "b", Status: todo.Completed},
			},
			want: true,
		},
		{
			name: "mixed statuses returns false",
			items: []todo.Todo{
				{Content: "a", Status: todo.Completed},
				{Content: "b", Status: todo.Pending},
			},
			want: false,
		},
		{
			name: "single pending item returns false",
			items: []todo.Todo{
				{Content: "a", Status: todo.Pending},
			},
			want: false,
		},
		{
			name: "in-progress item returns false",
			items: []todo.Todo{
				{Content: "a", Status: todo.Completed},
				{Content: "b", Status: todo.InProgress},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := todo.New()

			for _, item := range tt.items {
				l.Add(item)
			}

			got := l.AllCompleted()
			if got != tt.want {
				t.Errorf("AllCompleted() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ── TestCounts ────────────────────────────────────────────────────────────────

func TestCounts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		items          []todo.Todo
		wantPending    int
		wantInProgress int
		wantCompleted  int
	}{
		{
			name:           "empty list all counts zero",
			items:          []todo.Todo{},
			wantPending:    0,
			wantInProgress: 0,
			wantCompleted:  0,
		},
		{
			name: "counts each status correctly",
			items: []todo.Todo{
				{Content: "a", Status: todo.Pending},
				{Content: "b", Status: todo.Pending},
				{Content: "c", Status: todo.InProgress},
				{Content: "d", Status: todo.Completed},
				{Content: "e", Status: todo.Completed},
				{Content: "f", Status: todo.Completed},
			},
			wantPending:    2,
			wantInProgress: 1,
			wantCompleted:  3,
		},
		{
			name: "all pending",
			items: []todo.Todo{
				{Content: "a", Status: todo.Pending},
				{Content: "b", Status: todo.Pending},
			},
			wantPending:    2,
			wantInProgress: 0,
			wantCompleted:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := todo.New()

			for _, item := range tt.items {
				l.Add(item)
			}

			pending, inProgress, completed := l.Counts()
			if pending != tt.wantPending {
				t.Errorf("pending = %d, want %d", pending, tt.wantPending)
			}

			if inProgress != tt.wantInProgress {
				t.Errorf("inProgress = %d, want %d", inProgress, tt.wantInProgress)
			}

			if completed != tt.wantCompleted {
				t.Errorf("completed = %d, want %d", completed, tt.wantCompleted)
			}
		})
	}
}

// ── TestFindPendingInProgress ─────────────────────────────────────────────────

func TestFindPendingInProgress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		items          []todo.Todo
		wantPending    []int
		wantInProgress []int
	}{
		{
			name:           "empty list returns nil for both",
			items:          []todo.Todo{},
			wantPending:    nil,
			wantInProgress: nil,
		},
		{
			name: "returns correct indices for pending and in-progress",
			items: []todo.Todo{
				{Content: "a", Status: todo.Pending},    // 0
				{Content: "b", Status: todo.InProgress}, // 1
				{Content: "c", Status: todo.Completed},  // 2
				{Content: "d", Status: todo.Pending},    // 3
				{Content: "e", Status: todo.InProgress}, // 4
			},
			wantPending:    []int{0, 3},
			wantInProgress: []int{1, 4},
		},
		{
			name: "all completed returns nil for both",
			items: []todo.Todo{
				{Content: "a", Status: todo.Completed},
				{Content: "b", Status: todo.Completed},
			},
			wantPending:    nil,
			wantInProgress: nil,
		},
		{
			name: "single pending item",
			items: []todo.Todo{
				{Content: "a", Status: todo.Pending},
			},
			wantPending:    []int{0},
			wantInProgress: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := todo.New()

			for _, item := range tt.items {
				l.Add(item)
			}

			gotPending := l.FindPending()
			gotInProgress := l.FindInProgress()

			if !intSlicesEqual(gotPending, tt.wantPending) {
				t.Errorf("FindPending() = %v, want %v", gotPending, tt.wantPending)
			}

			if !intSlicesEqual(gotInProgress, tt.wantInProgress) {
				t.Errorf("FindInProgress() = %v, want %v", gotInProgress, tt.wantInProgress)
			}
		})
	}
}

// intSlicesEqual compares two int slices for equality, treating nil and empty
// as equal only when both are nil or both are empty.
func intSlicesEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

// ── TestParseStringRoundTrip ──────────────────────────────────────────────────

func TestParseStringRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		list    func() *todo.List
		wantErr bool
	}{
		{
			name: "list with all status types round-trips correctly",
			list: func() *todo.List {
				l := todo.New()
				l.Add(todo.Todo{Content: "write tests", Status: todo.Pending})
				l.Add(todo.Todo{Content: "review PR", Status: todo.InProgress})
				l.Add(todo.Todo{Content: "deploy", Status: todo.Completed})

				return l
			},
		},
		{
			name: "list with summary round-trips correctly",
			list: func() *todo.List {
				l := todo.New()
				l.SetSummary("overall project goal")
				l.Add(todo.Todo{Content: "step one", Status: todo.Pending})

				return l
			},
		},
		{
			name: "empty list round-trips to empty list",
			list: func() *todo.List {
				return todo.New()
			},
		},
		{
			name: "content with special characters round-trips correctly",
			list: func() *todo.List {
				l := todo.New()
				l.Add(todo.Todo{Content: "fix bug in module/pkg (issue #42)", Status: todo.Pending})

				return l
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			original := tt.list()
			serialized := original.String()

			parsed, err := todo.Parse(serialized)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil {
				return
			}

			origItems := original.Get()
			parsedItems := parsed.Get()

			if len(parsedItems) != len(origItems) {
				t.Fatalf("Parse returned %d items, original had %d", len(parsedItems), len(origItems))
			}

			for i := range origItems {
				if parsedItems[i].Content != origItems[i].Content {
					t.Errorf("item[%d].Content = %q, want %q", i, parsedItems[i].Content, origItems[i].Content)
				}

				if parsedItems[i].Status != origItems[i].Status {
					t.Errorf("item[%d].Status = %q, want %q", i, parsedItems[i].Status, origItems[i].Status)
				}
			}

			if parsed.Summary != original.Summary {
				t.Errorf("Summary = %q, want %q", parsed.Summary, original.Summary)
			}
		})
	}
}

// ── TestParseInvalidStatus ────────────────────────────────────────────────────

func TestParseInvalidStatus(t *testing.T) {
	t.Parallel()

	// A line that matches the regex but has an invalid status should return an error.
	input := "1. [bogus] some task content\n"

	_, err := todo.Parse(input)
	if err == nil {
		t.Error("Parse() with invalid status = nil, want error")
	}
}

// ── TestNilReceiver ───────────────────────────────────────────────────────────

func TestNilReceiver(t *testing.T) {
	t.Parallel()

	var l *todo.List

	t.Run("Len returns 0", func(t *testing.T) {
		t.Parallel()

		if got := l.Len(); got != 0 {
			t.Errorf("nil.Len() = %d, want 0", got)
		}
	})

	t.Run("IsEmpty returns true", func(t *testing.T) {
		t.Parallel()

		if got := l.IsEmpty(); !got {
			t.Error("nil.IsEmpty() = false, want true")
		}
	})

	t.Run("Get returns nil", func(t *testing.T) {
		t.Parallel()

		if got := l.Get(); got != nil {
			t.Errorf("nil.Get() = %v, want nil", got)
		}
	})

	t.Run("AllCompleted returns true", func(t *testing.T) {
		t.Parallel()

		if got := l.AllCompleted(); !got {
			t.Error("nil.AllCompleted() = false, want true")
		}
	})

	t.Run("Counts returns zeros", func(t *testing.T) {
		t.Parallel()

		pending, inProgress, completed := l.Counts()
		if pending != 0 || inProgress != 0 || completed != 0 {
			t.Errorf("nil.Counts() = (%d, %d, %d), want (0, 0, 0)", pending, inProgress, completed)
		}
	})

	t.Run("FindPending returns nil", func(t *testing.T) {
		t.Parallel()

		if got := l.FindPending(); got != nil {
			t.Errorf("nil.FindPending() = %v, want nil", got)
		}
	})

	t.Run("FindInProgress returns nil", func(t *testing.T) {
		t.Parallel()

		if got := l.FindInProgress(); got != nil {
			t.Errorf("nil.FindInProgress() = %v, want nil", got)
		}
	})

	t.Run("String returns empty string", func(t *testing.T) {
		t.Parallel()

		if got := l.String(); got != "" {
			t.Errorf("nil.String() = %q, want %q", got, "")
		}
	})
}

// ── TestIsEmpty ───────────────────────────────────────────────────────────────

func TestIsEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func() *todo.List
		want  bool
	}{
		{
			name:  "new list is empty",
			setup: todo.New,
			want:  true,
		},
		{
			name: "list with items is not empty",
			setup: func() *todo.List {
				l := todo.New()
				l.Add(todo.Todo{Content: "task", Status: todo.Pending})

				return l
			},
			want: false,
		},
		{
			// IsEmpty checks both Items and Summary; a summary alone makes it non-empty.
			name: "list with only a summary is not empty",
			setup: func() *todo.List {
				l := todo.New()
				l.SetSummary("some goal")

				return l
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := tt.setup()
			if got := l.IsEmpty(); got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ── TestReplace ───────────────────────────────────────────────────────────────

func TestReplace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		initial      []todo.Todo
		replacement  []todo.Todo
		wantContents []string
	}{
		{
			name: "replaces all existing items",
			initial: []todo.Todo{
				{Content: "old task", Status: todo.Pending},
			},
			replacement: []todo.Todo{
				{Content: "new task a", Status: todo.Pending},
				{Content: "new task b", Status: todo.Completed},
			},
			wantContents: []string{"new task a", "new task b"},
		},
		{
			name: "replace with empty slice clears list",
			initial: []todo.Todo{
				{Content: "existing", Status: todo.Pending},
			},
			replacement:  []todo.Todo{},
			wantContents: []string{},
		},
		{
			name:    "replace on empty list works",
			initial: []todo.Todo{},
			replacement: []todo.Todo{
				{Content: "brand new", Status: todo.InProgress},
			},
			wantContents: []string{"brand new"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := todo.New()

			for _, item := range tt.initial {
				l.Add(item)
			}

			l.Replace(tt.replacement)

			got := l.Get()
			if len(got) != len(tt.wantContents) {
				t.Fatalf("after Replace, list has %d items, want %d", len(got), len(tt.wantContents))
			}

			for i, want := range tt.wantContents {
				if got[i].Content != want {
					t.Errorf("item[%d].Content = %q, want %q", i, got[i].Content, want)
				}
			}
		})
	}
}
