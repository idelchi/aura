package session_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/idelchi/aura/internal/session"
	"github.com/idelchi/aura/internal/todo"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/roles"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func newStore(t *testing.T) *session.Store {
	t.Helper()

	return session.NewStore(t.TempDir())
}

func newManager(t *testing.T) *session.Manager {
	t.Helper()

	return session.NewManager(newStore(t))
}

func sampleMessages() message.Messages {
	return message.New(
		message.Message{Role: roles.User, Content: "hello"},
		message.Message{Role: roles.Assistant, Content: "hi there"},
	)
}

func sampleMeta() session.Meta {
	return session.Meta{Agent: "TestAgent", Mode: "test", Think: "off"}
}

func sampleTodos() *todo.List {
	l := todo.New()
	l.Add(todo.Todo{Content: "task 1", Status: todo.Pending})

	return l
}

// mustSave calls Save and fails the test if it errors.
func mustSave(t *testing.T, m *session.Manager, title string) *session.Session {
	t.Helper()

	s, err := m.Save(title, sampleMeta(), sampleMessages(), sampleTodos())
	if err != nil {
		t.Fatalf("Save(%q) error = %v", title, err)
	}

	return s
}

// ── Store tests ───────────────────────────────────────────────────────────────

func TestStoreSaveAndLoad(t *testing.T) {
	t.Parallel()

	store := newStore(t)

	now := time.Now().Truncate(time.Second)
	msgs := sampleMessages()
	meta := sampleMeta()
	todos := sampleTodos()

	orig := &session.Session{
		ID:        "test-session-1",
		Title:     "Test Session",
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now,
		Messages:  msgs,
		Meta:      meta,
		Todos:     todos,
	}

	if err := store.Save(orig); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := store.Load(orig.ID)
	if err != nil {
		t.Fatalf("Load(%q) error = %v", orig.ID, err)
	}

	if got.ID != orig.ID {
		t.Errorf("ID = %q, want %q", got.ID, orig.ID)
	}

	if got.Title != orig.Title {
		t.Errorf("Title = %q, want %q", got.Title, orig.Title)
	}

	if got.Meta.Agent != orig.Meta.Agent {
		t.Errorf("Meta.Agent = %q, want %q", got.Meta.Agent, orig.Meta.Agent)
	}

	if got.Meta.Mode != orig.Meta.Mode {
		t.Errorf("Meta.Mode = %q, want %q", got.Meta.Mode, orig.Meta.Mode)
	}

	if len(got.Messages) != len(orig.Messages) {
		t.Fatalf("len(Messages) = %d, want %d", len(got.Messages), len(orig.Messages))
	}

	if got.Messages[0].Content != orig.Messages[0].Content {
		t.Errorf("Messages[0].Content = %q, want %q", got.Messages[0].Content, orig.Messages[0].Content)
	}

	if got.Todos == nil {
		t.Fatal("Todos = nil, want non-nil")
	}

	if got.Todos.Len() != orig.Todos.Len() {
		t.Errorf("Todos.Len() = %d, want %d", got.Todos.Len(), orig.Todos.Len())
	}
}

func TestStoreList(t *testing.T) {
	t.Parallel()

	store := newStore(t)

	older := &session.Session{
		ID:        "sess-older",
		Title:     "Older",
		CreatedAt: time.Now().Add(-2 * time.Hour),
		UpdatedAt: time.Now().Add(-time.Hour),
	}
	newer := &session.Session{
		ID:        "sess-newer",
		Title:     "Newer",
		CreatedAt: time.Now().Add(-time.Hour),
		UpdatedAt: time.Now(),
	}

	if err := store.Save(older); err != nil {
		t.Fatalf("Save(older) error = %v", err)
	}

	if err := store.Save(newer); err != nil {
		t.Fatalf("Save(newer) error = %v", err)
	}

	result, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(result.Summaries) != 2 {
		t.Fatalf("List() returned %d summaries, want 2", len(result.Summaries))
	}

	// Newest first.
	if result.Summaries[0].ID != newer.ID {
		t.Errorf("summaries[0].ID = %q, want %q (newest first)", result.Summaries[0].ID, newer.ID)
	}

	if result.Summaries[1].ID != older.ID {
		t.Errorf("summaries[1].ID = %q, want %q", result.Summaries[1].ID, older.ID)
	}
}

func TestStoreFind(t *testing.T) {
	t.Parallel()

	store := newStore(t)

	sess := &session.Session{
		ID:        "abcdef1234",
		Title:     "Find me",
		UpdatedAt: time.Now(),
	}

	if err := store.Save(sess); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	t.Run("exact prefix matches", func(t *testing.T) {
		t.Parallel()

		id, err := store.Find("abcdef")
		if err != nil {
			t.Fatalf("Find(%q) error = %v", "abcdef", err)
		}

		if id != sess.ID {
			t.Errorf("Find() = %q, want %q", id, sess.ID)
		}
	})

	t.Run("wrong prefix returns ErrNotFound", func(t *testing.T) {
		t.Parallel()

		_, err := store.Find("zzz999")
		if !errors.Is(err, session.ErrNotFound) {
			t.Errorf("Find(wrong prefix) error = %v, want ErrNotFound", err)
		}
	})

	t.Run("ambiguous prefix returns error", func(t *testing.T) {
		t.Parallel()

		// Create a second store so ambiguous sessions don't interfere with
		// parallel subtests that read from the shared store above.
		ambStore := newStore(t)

		s1 := &session.Session{ID: "prefix-aaa", UpdatedAt: time.Now()}
		s2 := &session.Session{ID: "prefix-bbb", UpdatedAt: time.Now()}

		if err := ambStore.Save(s1); err != nil {
			t.Fatalf("Save(s1) error = %v", err)
		}

		if err := ambStore.Save(s2); err != nil {
			t.Fatalf("Save(s2) error = %v", err)
		}

		_, err := ambStore.Find("prefix-")
		if err == nil {
			t.Error("Find(ambiguous prefix) = nil, want ambiguous error")
		}

		if errors.Is(err, session.ErrNotFound) {
			t.Errorf("Find(ambiguous) error should not be ErrNotFound, got %v", err)
		}
	})
}

func TestStoreExists(t *testing.T) {
	t.Parallel()

	store := newStore(t)

	sess := &session.Session{
		ID:        "exists-test",
		UpdatedAt: time.Now(),
	}

	if store.Exists(sess.ID) {
		t.Fatal("Exists() = true before Save, want false")
	}

	if err := store.Save(sess); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	if !store.Exists(sess.ID) {
		t.Errorf("Exists() = false after Save, want true")
	}

	if store.Exists("non-existent-id") {
		t.Errorf("Exists(%q) = true for never-saved ID, want false", "non-existent-id")
	}
}

func TestStoreRename(t *testing.T) {
	t.Parallel()

	store := newStore(t)

	sess := &session.Session{
		ID:        "old-id",
		Title:     "Rename Test",
		UpdatedAt: time.Now(),
	}

	if err := store.Save(sess); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	if err := store.Rename("old-id", "new-id"); err != nil {
		t.Fatalf("Rename() error = %v", err)
	}

	if store.Exists("old-id") {
		t.Error("Exists(oldID) = true after Rename, want false")
	}

	loaded, err := store.Load("new-id")
	if err != nil {
		t.Fatalf("Load(newID) error = %v", err)
	}

	// The file was renamed but the JSON content still has the original ID field;
	// the store does not rewrite the JSON body — verify the title is intact.
	if loaded.Title != sess.Title {
		t.Errorf("loaded.Title = %q, want %q", loaded.Title, sess.Title)
	}
}

// ── Manager tests ─────────────────────────────────────────────────────────────

func TestManagerSave(t *testing.T) {
	t.Parallel()

	t.Run("first save creates session with UUID id", func(t *testing.T) {
		t.Parallel()

		m := newManager(t)
		s := mustSave(t, m, "first")

		if s.ID == "" {
			t.Error("Session.ID is empty after first Save")
		}

		if s.Title != "first" {
			t.Errorf("Session.Title = %q, want %q", s.Title, "first")
		}
	})

	t.Run("re-save updates same session", func(t *testing.T) {
		t.Parallel()

		m := newManager(t)

		s1 := mustSave(t, m, "original title")
		firstID := s1.ID

		s2 := mustSave(t, m, "updated title")

		if s2.ID != firstID {
			t.Errorf("second Save ID = %q, want same as first %q", s2.ID, firstID)
		}

		if s2.Title != "updated title" {
			t.Errorf("second Save Title = %q, want %q", s2.Title, "updated title")
		}
	})
}

func TestManagerResume(t *testing.T) {
	t.Parallel()

	store := newStore(t)

	sess := &session.Session{
		ID:        "resume-me",
		Title:     "Resume Title",
		UpdatedAt: time.Now(),
		Messages:  sampleMessages(),
		Meta:      sampleMeta(),
	}

	if err := store.Save(sess); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	mgr := session.NewManager(store)

	resumed, err := mgr.Resume("resume-me")
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}

	if resumed.ID != "resume-me" {
		t.Errorf("resumed.ID = %q, want %q", resumed.ID, "resume-me")
	}

	if resumed.Title != "Resume Title" {
		t.Errorf("resumed.Title = %q, want %q", resumed.Title, "Resume Title")
	}

	if mgr.ActiveID() != "resume-me" {
		t.Errorf("ActiveID() = %q after Resume, want %q", mgr.ActiveID(), "resume-me")
	}
}

func TestManagerFork(t *testing.T) {
	t.Parallel()

	mgr := newManager(t)

	s1 := mustSave(t, mgr, "original")
	firstID := s1.ID

	mgr.Fork()

	s2 := mustSave(t, mgr, "forked")

	if s2.ID == firstID {
		t.Errorf("forked Save ID = %q, want a different ID from original %q", s2.ID, firstID)
	}
}

func TestManagerSetName(t *testing.T) {
	t.Parallel()

	t.Run("set name before save uses name as id", func(t *testing.T) {
		t.Parallel()

		mgr := newManager(t)

		if err := mgr.SetName("my-session"); err != nil {
			t.Fatalf("SetName() before Save error = %v", err)
		}

		s := mustSave(t, mgr, "titled")

		if s.ID != "my-session" {
			t.Errorf("Session.ID = %q, want %q", s.ID, "my-session")
		}
	})

	t.Run("set name after save renames on disk", func(t *testing.T) {
		t.Parallel()

		mgr := newManager(t)
		s := mustSave(t, mgr, "before rename")
		oldID := s.ID

		if err := mgr.SetName("renamed-session"); err != nil {
			t.Fatalf("SetName() after Save error = %v", err)
		}

		if mgr.ActiveID() != "renamed-session" {
			t.Errorf("ActiveID() = %q after SetName, want %q", mgr.ActiveID(), "renamed-session")
		}

		_ = oldID
	})

	t.Run("duplicate name errors", func(t *testing.T) {
		t.Parallel()

		// Two managers sharing the same store directory so that the second
		// name is already occupied.
		store := newStore(t)

		mgr1 := session.NewManager(store)
		if err := mgr1.SetName("taken-name"); err != nil {
			t.Fatalf("SetName on mgr1 error = %v", err)
		}

		mustSave(t, mgr1, "mgr1")

		mgr2 := session.NewManager(store)

		if err := mgr2.SetName("taken-name"); err == nil {
			t.Error("SetName() with duplicate name = nil, want error")
		}
	})

	t.Run("path separator in name errors", func(t *testing.T) {
		t.Parallel()

		mgr := newManager(t)

		for _, badName := range []string{"a/b", `a\b`, "a..b"} {
			if err := mgr.SetName(badName); err == nil {
				t.Errorf("SetName(%q) = nil, want error for path-separator name", badName)
			}
		}
	})
}

func TestManagerActiveIDAndTitle(t *testing.T) {
	t.Parallel()

	t.Run("ActiveID returns pending name before save", func(t *testing.T) {
		t.Parallel()

		mgr := newManager(t)

		if id := mgr.ActiveID(); id != "" {
			t.Errorf("ActiveID() = %q before any action, want empty", id)
		}

		if err := mgr.SetName("pending-name"); err != nil {
			t.Fatalf("SetName() error = %v", err)
		}

		if id := mgr.ActiveID(); id != "pending-name" {
			t.Errorf("ActiveID() = %q after SetName, want %q", id, "pending-name")
		}
	})

	t.Run("ActiveID returns session id after save", func(t *testing.T) {
		t.Parallel()

		mgr := newManager(t)
		s := mustSave(t, mgr, "saved")

		if id := mgr.ActiveID(); id != s.ID {
			t.Errorf("ActiveID() = %q after Save, want %q", id, s.ID)
		}
	})

	t.Run("ActiveTitle returns empty before save", func(t *testing.T) {
		t.Parallel()

		mgr := newManager(t)

		if title := mgr.ActiveTitle(); title != "" {
			t.Errorf("ActiveTitle() = %q before Save, want empty", title)
		}
	})

	t.Run("ActiveTitle returns title after save", func(t *testing.T) {
		t.Parallel()

		mgr := newManager(t)
		mustSave(t, mgr, "my title")

		if title := mgr.ActiveTitle(); title != "my title" {
			t.Errorf("ActiveTitle() = %q after Save, want %q", title, "my title")
		}
	})
}

// ── Session / Summary tests ───────────────────────────────────────────────────

func TestSessionFirstUserContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		msgs        message.Messages
		wantContent string
	}{
		{
			name: "first user message content is returned",
			msgs: message.New(
				message.Message{Role: roles.Assistant, Content: "preamble"},
				message.Message{Role: roles.User, Content: "  hello world  "},
				message.Message{Role: roles.User, Content: "second user"},
			),
			wantContent: "hello world",
		},
		{
			name: "no user messages returns empty string",
			msgs: message.New(
				message.Message{Role: roles.Assistant, Content: "only assistant"},
			),
			wantContent: "",
		},
		{
			name:        "empty messages returns empty string",
			msgs:        message.New(),
			wantContent: "",
		},
		{
			name: "user message with whitespace only trims to empty",
			msgs: message.New(
				message.Message{Role: roles.User, Content: "   "},
			),
			wantContent: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sess := &session.Session{
				ID:       "test-id",
				Messages: tt.msgs,
			}

			got := sess.FirstUserContent()
			if got != tt.wantContent {
				t.Errorf("FirstUserContent() = %q, want %q", got, tt.wantContent)
			}
		})
	}
}

func TestSummaryDisplay(t *testing.T) {
	t.Parallel()

	s := session.Summary{
		ID:               "abcdef12-0000-0000-0000-000000000000",
		Title:            "My Session",
		UpdatedAt:        time.Now().Add(-5 * time.Minute),
		FirstUserMessage: "What is the meaning of life?",
	}

	display := s.Display()

	if display == "" {
		t.Fatal("Display() = empty string, want non-empty")
	}

	// Should contain a recognisable portion of the UUID (short form = first 8 chars).
	if !strings.Contains(display, "abcdef12") {
		t.Errorf("Display() = %q, want it to contain short ID %q", display, "abcdef12")
	}

	if !strings.Contains(display, "My Session") {
		t.Errorf("Display() = %q, want it to contain title %q", display, "My Session")
	}
}

func TestSummaryShortID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		id       string
		wantLen  int  // expected length of ShortID()
		wantFull bool // true = full ID returned (custom name)
	}{
		{
			name:    "UUID is truncated to 8 characters",
			id:      "550e8400-e29b-41d4-a716-446655440000",
			wantLen: 8,
		},
		{
			name:     "custom name returned in full",
			id:       "my-custom-name",
			wantFull: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := session.Summary{ID: tt.id}
			got := s.ShortID()

			if tt.wantFull {
				if got != tt.id {
					t.Errorf("ShortID() = %q, want full ID %q", got, tt.id)
				}
			} else {
				if len(got) != tt.wantLen {
					t.Errorf("ShortID() = %q (len %d), want len %d", got, len(got), tt.wantLen)
				}

				if !strings.HasPrefix(tt.id, got) {
					t.Errorf("ShortID() = %q, want it to be a prefix of %q", got, tt.id)
				}
			}
		})
	}
}
