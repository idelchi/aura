package session_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/idelchi/aura/internal/session"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/roles"
)

// seed stores a session with a system prompt and two conversation messages.
func seed(t *testing.T, store *session.Store, id string) string {
	t.Helper()

	sess := &session.Session{
		ID:        id,
		Title:     "Seed " + id,
		CreatedAt: time.Now().Add(-time.Hour),
		UpdatedAt: time.Now().Add(-time.Hour),
		Messages: message.New(
			message.Message{Role: roles.System, Content: "system"},
			message.Message{Role: roles.User, Content: "teach me"},
			message.Message{Role: roles.Assistant, Content: "lesson"},
		),
		Meta: sampleMeta(),
	}

	rev, err := store.Save(sess)
	if err != nil {
		t.Fatalf("Save(%s): %v", id, err)
	}

	return rev
}

func fileBytes(t *testing.T, store *session.Store, id string) []byte {
	t.Helper()

	data, err := os.ReadFile(store.Path(id).Path())
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func TestStoreSaveIsAtomicAndReportsRevision(t *testing.T) {
	t.Parallel()

	store := newStore(t)
	rev := seed(t, store, "atomic")

	if got := session.Digest(fileBytes(t, store, "atomic")); got != rev {
		t.Fatalf("revision %s does not match stored bytes %s", rev, got)
	}

	entries, err := os.ReadDir(store.Dir())
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range entries {
		if e.Type().IsRegular() && e.Name() != "atomic.json" {
			t.Errorf("leftover file after atomic save: %s", e.Name())
		}
	}

	info, err := os.Stat(store.Path("atomic").Path())
	if err != nil {
		t.Fatal(err)
	}

	if perm := info.Mode().Perm(); runtime.GOOS != "windows" && perm != 0o600 {
		t.Errorf("session file mode = %v, want 0600", perm)
	}
}

func TestStoreForkKeepsParentAndLineage(t *testing.T) {
	t.Parallel()

	store := newStore(t)
	parentRev := seed(t, store, "parent")
	before := fileBytes(t, store, "parent")

	got, err := store.Fork("parent", "child", "", parentRev)
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}

	if !got.Created || got.Session.ID != "child" || got.Session.Parent == nil ||
		*got.Session.Parent != (session.Lineage{ID: "parent", Revision: parentRev}) {
		t.Fatalf("unexpected fork result: %+v", got)
	}

	if string(fileBytes(t, store, "parent")) != string(before) {
		t.Fatal("fork modified the parent file")
	}

	child, err := store.Load("child")
	if err != nil {
		t.Fatal(err)
	}

	if child.Title != "Seed parent" || child.ConversationLength() != 2 || child.Frozen {
		t.Fatalf("child = %+v", child)
	}

	again, err := store.Fork("parent", "child", "", "")
	if err != nil || again.Created || again.Session.Revision != got.Session.Revision {
		t.Fatalf("repeated fork = %+v, %v; want existing child unchanged", again, err)
	}

	if _, err := store.Fork("parent", "child2", "", strings.Repeat("0", 64)); !errors.Is(err, session.ErrRevisionMismatch) {
		t.Fatalf("fork with stale revision: %v", err)
	}

	seed(t, store, "other")

	if _, err := store.Fork("parent", "other", "", ""); !errors.Is(err, session.ErrExists) {
		t.Fatalf("fork onto unrelated session: %v", err)
	}

	if _, err := store.Fork("missing", "x", "", ""); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("fork of missing parent: %v", err)
	}

	if _, err := store.Fork("parent", "../escape", "", ""); !errors.Is(err, session.ErrInvalidID) {
		t.Fatalf("fork to invalid id: %v", err)
	}
}

func TestStoreFreezeIsPermanentForManagers(t *testing.T) {
	t.Parallel()

	store := newStore(t)
	rev := seed(t, store, "baseline")

	frozen, err := store.Freeze("baseline", rev)
	if err != nil || !frozen.Changed || !frozen.Session.Frozen {
		t.Fatalf("Freeze = %+v, %v", frozen, err)
	}

	again, err := store.Freeze("baseline", "")
	if err != nil || again.Changed || again.Session.Revision != frozen.Session.Revision {
		t.Fatalf("second Freeze = %+v, %v", again, err)
	}

	if _, err := session.NewManager(store).Resume("baseline"); !errors.Is(err, session.ErrFrozen) {
		t.Fatalf("Resume(frozen) = %v, want ErrFrozen", err)
	}

	child, err := store.Fork("baseline", "task", "", frozen.Session.Revision)
	if err != nil || child.Session.Frozen {
		t.Fatalf("fork of frozen parent = %+v, %v", child, err)
	}

	if _, err := session.NewManager(store).Resume("task"); err != nil {
		t.Fatalf("Resume(fork) = %v", err)
	}
}

func TestStoreImportExportIdempotent(t *testing.T) {
	t.Parallel()

	source := newStore(t)
	seed(t, source, "native")
	path := source.Path("native").Path()

	store := newStore(t)

	first, err := store.Import(path, "", true)
	if err != nil || !first.Created || !first.FrozenNow || !first.Session.Frozen {
		t.Fatalf("Import = %+v, %v", first, err)
	}

	if first.SourceSHA256 != session.Digest(fileBytes(t, source, "native")) {
		t.Fatal("source digest does not describe the input file")
	}

	second, err := store.Import(path, "", true)
	if err != nil || second.Created || second.FrozenNow || second.Session.Revision != first.Session.Revision {
		t.Fatalf("repeated Import = %+v, %v", second, err)
	}

	renamed, err := store.Import(path, "copy", false)
	if err != nil || !renamed.Created || renamed.Session.ID != "copy" || renamed.Session.Frozen {
		t.Fatalf("Import --id = %+v, %v", renamed, err)
	}

	different := newStore(t)
	seed(t, different, "native")

	var changed session.Session
	if err := json.Unmarshal(fileBytes(t, different, "native"), &changed); err != nil {
		t.Fatal(err)
	}

	changed.Title = "edited"
	if _, err := different.Save(&changed); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Import(different.Path("native").Path(), "", false); !errors.Is(err, session.ErrExists) {
		t.Fatalf("Import of a different conversation = %v, want ErrExists", err)
	}

	out := filepath.Join(t.TempDir(), "export.json")

	exported, err := store.Export("native", out)
	if err != nil || !exported.Written || exported.SHA256 != first.Session.Revision {
		t.Fatalf("Export = %+v, %v", exported, err)
	}

	again, err := store.Export("native", out)
	if err != nil || again.Written {
		t.Fatalf("repeated Export = %+v, %v", again, err)
	}

	if err := os.WriteFile(out, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Export("native", out); !errors.Is(err, session.ErrExists) {
		t.Fatalf("Export over different file = %v, want ErrExists", err)
	}

	if _, err := store.Import(out, "broken", false); !errors.Is(err, session.ErrInvalidSession) {
		t.Fatalf("Import of a non-session = %v, want ErrInvalidSession", err)
	}
}

func TestLeaseExcludesConcurrentWriters(t *testing.T) {
	t.Parallel()

	store := newStore(t)
	seed(t, store, "busy")

	holder := session.NewManager(store)
	if _, err := holder.Resume("busy"); err != nil {
		t.Fatal(err)
	}

	if _, err := session.NewManager(store).Resume("busy"); !errors.Is(err, session.ErrLocked) {
		t.Fatalf("second Resume = %v, want ErrLocked", err)
	}

	if _, err := store.Freeze("busy", ""); !errors.Is(err, session.ErrLocked) {
		t.Fatalf("Freeze of held session = %v, want ErrLocked", err)
	}

	holder.Close()

	if _, err := session.NewManager(store).Resume("busy"); err != nil {
		t.Fatalf("Resume after release = %v", err)
	}
}

func TestManagerSaveSkipsUnchangedContent(t *testing.T) {
	t.Parallel()

	m := newManager(t)

	first, err := m.Save("t", sampleMeta(), sampleMessages(), sampleTodos())
	if err != nil || !first.Changed {
		t.Fatalf("first Save = %+v, %v", first, err)
	}

	same, err := m.Save("t", sampleMeta(), sampleMessages(), sampleTodos())
	if err != nil || same.Changed || same.Revision != first.Revision {
		t.Fatalf("unchanged Save = %+v, %v", same, err)
	}

	more := append(sampleMessages(), message.Message{Role: roles.User, Content: "again"})

	next, err := m.Save("t", sampleMeta(), more, sampleTodos())
	if err != nil || !next.Changed || next.Revision == first.Revision {
		t.Fatalf("changed Save = %+v, %v", next, err)
	}
}

func TestManagerBeginReservesID(t *testing.T) {
	t.Parallel()

	store := newStore(t)
	m := session.NewManager(store)

	id, err := m.Begin()
	if err != nil || id == "" {
		t.Fatalf("Begin = %q, %v", id, err)
	}

	if _, err := store.Acquire(id); !errors.Is(err, session.ErrLocked) {
		t.Fatalf("reserved id not leased: %v", err)
	}

	saved, err := m.Save("", sampleMeta(), message.New(), nil)
	if err != nil || saved.Session.ID != id || !store.Exists(id) {
		t.Fatalf("Save after Begin = %+v, %v", saved, err)
	}
}

func TestStoreFindPrefersExactID(t *testing.T) {
	t.Parallel()

	store := newStore(t)
	seed(t, store, "child")
	seed(t, store, "child-2")

	if id, err := store.Find("child"); err != nil || id != "child" {
		t.Fatalf("Find(child) = %q, %v", id, err)
	}
}
