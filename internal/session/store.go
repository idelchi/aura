package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/idelchi/aura/internal/atomicfile"
	"github.com/idelchi/aura/pkg/automation"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// ListResult holds session summaries and any warnings encountered during listing.
type ListResult struct {
	Summaries []Summary
	Sessions  []Info
	Warnings  []string
}

// Results of store operations, as printed by `aura session --json`.
type (
	ForkResult   = automation.ForkResult
	FreezeResult = automation.FreezeResult
	ImportResult = automation.ImportResult
	ExportResult = automation.ExportResult
)

// Store handles JSON file-based session persistence.
//
// Every write is atomic: a temporary file in the store directory is written, fsynced and
// replaced over the session file using the platform's publication operation. Unix also syncs the directory.
// A session is written only by the holder of its Lease; the Store's own mutating operations take the lease themselves.
type Store struct {
	dir folder.Folder
}

// NewStore creates a Store rooted at the given directory.
func NewStore(dir string) *Store {
	return &Store{dir: folder.New(dir)}
}

// Dir returns the store directory.
func (s *Store) Dir() string {
	return s.dir.Path()
}

// Save writes a session atomically and returns the revision of the stored bytes.
func (s *Store) Save(session *Session) (string, error) {
	if err := session.Validate(); err != nil {
		return "", err
	}

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling session: %w", err)
	}

	return s.write(session.ID, data)
}

// write atomically replaces the session file for id with data.
func (s *Store) write(id string, data []byte) (string, error) {
	if err := s.dir.Create(); err != nil {
		return "", fmt.Errorf("creating session directory: %w", err)
	}

	if err := atomicfile.WriteFile(s.Path(id).Path(), data); err != nil {
		return "", fmt.Errorf("writing session %q: %w", id, err)
	}

	return Digest(data), nil
}

// Load reads a session from disk by exact ID.
func (s *Store) Load(id string) (*Session, error) {
	sess, _, err := s.LoadRevision(id)

	return sess, err
}

// LoadRevision reads a session by exact ID together with the revision of its stored bytes.
func (s *Store) LoadRevision(id string) (*Session, string, error) {
	data, err := s.raw(id)
	if err != nil {
		return nil, "", err
	}

	sess, err := decode(data)
	if err != nil {
		return nil, "", err
	}

	if sess.ID != id {
		return nil, "", fmt.Errorf("%w: file %s contains session %q", ErrInvalidSession, s.Path(id).Base(), sess.ID)
	}

	return sess, Digest(data), nil
}

// Info describes the stored session with the given exact ID.
func (s *Store) Info(id string) (Info, error) {
	sess, rev, err := s.LoadRevision(id)
	if err != nil {
		return Info{}, err
	}

	return sess.Info(rev), nil
}

// raw returns the stored bytes for id.
func (s *Store) raw(id string) ([]byte, error) {
	if err := ValidateID(id); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(s.Path(id).Path())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %q", ErrNotFound, id)
	}

	if err != nil {
		return nil, fmt.Errorf("reading session: %w", err)
	}

	return data, nil
}

// decode parses and validates stored session bytes.
func decode(data []byte) (*Session, error) {
	var session Session

	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidSession, err)
	}

	if err := session.Validate(); err != nil {
		return nil, err
	}

	return &session, nil
}

// List returns summaries of all sessions, sorted newest first.
// Corrupt or unreadable session files are reported as warnings rather than
// causing the entire listing to fail.
func (s *Store) List() (ListResult, error) {
	if !s.dir.Exists() {
		return ListResult{}, nil
	}

	listing, err := s.dir.ListFiles()
	if err != nil {
		return ListResult{}, fmt.Errorf("reading session directory: %w", err)
	}

	var result ListResult

	for _, f := range listing {
		if f.Extension() != "json" {
			continue
		}

		data, err := f.Read()
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("skipping unreadable session %s: %v", f.Base(), err))

			continue
		}

		session, err := decode(data)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("skipping corrupt session %s: %v", f.Base(), err))

			continue
		}

		result.Summaries = append(result.Summaries, Summary{
			ID:               session.ID,
			Title:            session.Title,
			UpdatedAt:        session.UpdatedAt,
			FirstUserMessage: session.FirstUserContent(),
		})
		result.Sessions = append(result.Sessions, session.Info(Digest(data)))
	}

	newestFirst := func(a, b time.Time) int { return b.Compare(a) }

	slices.SortFunc(result.Summaries, func(a, b Summary) int { return newestFirst(a.UpdatedAt, b.UpdatedAt) })
	slices.SortFunc(result.Sessions, func(a, b Info) int { return newestFirst(a.UpdatedAt, b.UpdatedAt) })

	return result, nil
}

// Find resolves a session ID prefix to a full session ID.
// An exact ID always wins; otherwise exactly one session must start with the prefix.
func (s *Store) Find(prefix string) (string, error) {
	if ValidateID(prefix) == nil && s.Exists(prefix) {
		return prefix, nil
	}

	result, err := s.List()
	if err != nil {
		return "", err
	}

	var matches []string

	for _, sm := range result.Summaries {
		if strings.HasPrefix(sm.ID, prefix) {
			matches = append(matches, sm.ID)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no session matching %q: %w", prefix, ErrNotFound)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("ambiguous prefix %q matches %d sessions", prefix, len(matches))
	}
}

// Exists reports whether a session with the given ID exists on disk.
func (s *Store) Exists(id string) bool {
	return s.Path(id).Exists()
}

// Rename moves a session to a new ID, rewriting its stored ID, and returns the new revision.
// The caller must hold the leases of both IDs.
func (s *Store) Rename(oldID, newID string) (string, error) {
	if err := ValidateID(newID); err != nil {
		return "", err
	}

	if s.Exists(newID) {
		return "", fmt.Errorf("%w: %q", ErrExists, newID)
	}

	data, err := s.raw(oldID)
	if err != nil {
		return "", err
	}

	data, err = patch(data, map[string]any{"id": newID})
	if err != nil {
		return "", err
	}

	rev, err := s.write(newID, data)
	if err != nil {
		return "", err
	}

	if err := os.Remove(s.Path(oldID).Path()); err != nil {
		return "", fmt.Errorf("removing renamed session: %w", err)
	}

	return rev, nil
}

// Fork stores a copy of the parent as childID with the parent's exact stored revision as lineage.
// The parent is only read. An existing child forked from the same parent revision is returned
// unchanged (Created false); any other existing child is ErrExists.
func (s *Store) Fork(parentID, childID, title, expectRevision string) (ForkResult, error) {
	if err := ValidateID(childID); err != nil {
		return ForkResult{}, err
	}

	if childID == parentID {
		return ForkResult{}, fmt.Errorf("%w: fork %q onto itself", ErrExists, parentID)
	}

	parentData, err := s.raw(parentID)
	if err != nil {
		return ForkResult{}, err
	}

	parentRev := Digest(parentData)

	if expectRevision != "" && expectRevision != parentRev {
		return ForkResult{}, fmt.Errorf("%w: %q is %s, expected %s", ErrRevisionMismatch, parentID, parentRev, expectRevision)
	}

	parent, err := decode(parentData)
	if err != nil {
		return ForkResult{}, err
	}

	lease, err := s.Acquire(childID)
	if err != nil {
		return ForkResult{}, err
	}

	defer lease.Release()

	lineage := Lineage{ID: parentID, Revision: parentRev}

	if s.Exists(childID) {
		existing, rev, err := s.LoadRevision(childID)
		if err != nil {
			return ForkResult{}, err
		}

		if existing.Parent == nil || *existing.Parent != lineage {
			return ForkResult{}, fmt.Errorf("%w: %q is not a fork of %s@%s", ErrExists, childID, parentID, parentRev)
		}

		return ForkResult{Session: existing.Info(rev), Parent: parent.Info(parentRev), Created: false}, nil
	}

	if title == "" {
		title = parent.Title
	}

	now := time.Now().UTC()

	data, err := patch(parentData, map[string]any{
		"id":         childID,
		"title":      title,
		"created_at": now,
		"updated_at": now,
		"frozen":     nil,
		"parent":     lineage,
	})
	if err != nil {
		return ForkResult{}, err
	}

	child, err := decode(data)
	if err != nil {
		return ForkResult{}, err
	}

	if err := sameConversation(parent, child); err != nil {
		return ForkResult{}, err
	}

	rev, err := s.write(childID, data)
	if err != nil {
		return ForkResult{}, err
	}

	return ForkResult{Session: child.Info(rev), Parent: parent.Info(parentRev), Created: true}, nil
}

// Freeze irreversibly marks a session immutable. Freezing a frozen session changes nothing.
func (s *Store) Freeze(id, expectRevision string) (FreezeResult, error) {
	lease, err := s.Acquire(id)
	if err != nil {
		return FreezeResult{}, err
	}

	defer lease.Release()

	data, err := s.raw(id)
	if err != nil {
		return FreezeResult{}, err
	}

	rev := Digest(data)

	if expectRevision != "" && expectRevision != rev {
		return FreezeResult{}, fmt.Errorf("%w: %q is %s, expected %s", ErrRevisionMismatch, id, rev, expectRevision)
	}

	sess, err := decode(data)
	if err != nil {
		return FreezeResult{}, err
	}

	if sess.Frozen {
		return FreezeResult{Session: sess.Info(rev), Changed: false}, nil
	}

	info, err := s.freeze(id, data)
	if err != nil {
		return FreezeResult{}, err
	}

	return FreezeResult{Session: info, Changed: true}, nil
}

// freeze stores data with the frozen flag set. The caller holds the lease.
func (s *Store) freeze(id string, data []byte) (Info, error) {
	data, err := patch(data, map[string]any{"frozen": true})
	if err != nil {
		return Info{}, err
	}

	sess, err := decode(data)
	if err != nil {
		return Info{}, err
	}

	rev, err := s.write(id, data)
	if err != nil {
		return Info{}, err
	}

	return sess.Info(rev), nil
}

// Import stores a native session file under its own ID or the given one.
// Importing the same conversation again is a no-op (optionally freezing it);
// a different conversation under the target ID is ErrExists.
func (s *Store) Import(path, id string, freeze bool) (ImportResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ImportResult{}, fmt.Errorf("reading import: %w", err)
	}

	source, err := decode(data)
	if err != nil {
		return ImportResult{}, err
	}

	target := source.ID
	if id != "" {
		target = id
	}

	if err := ValidateID(target); err != nil {
		return ImportResult{}, err
	}

	result := ImportResult{SourceSHA256: Digest(data)}

	lease, err := s.Acquire(target)
	if err != nil {
		return ImportResult{}, err
	}

	defer lease.Release()

	if s.Exists(target) {
		existingData, err := s.raw(target)
		if err != nil {
			return ImportResult{}, err
		}

		existing, err := decode(existingData)
		if err != nil {
			return ImportResult{}, err
		}

		same, err := sameContent(source, existing)
		if err != nil {
			return ImportResult{}, err
		}

		if !same {
			return ImportResult{}, fmt.Errorf("%w: %q holds a different conversation", ErrExists, target)
		}

		result.Session = existing.Info(Digest(existingData))

		if freeze && !existing.Frozen {
			info, err := s.freeze(target, existingData)
			if err != nil {
				return ImportResult{}, err
			}

			result.Session, result.FrozenNow = info, true
		}

		return result, nil
	}

	fields := map[string]any{"id": target}
	if freeze {
		fields["frozen"] = true
	}

	data, err = patch(data, fields)
	if err != nil {
		return ImportResult{}, err
	}

	imported, err := decode(data)
	if err != nil {
		return ImportResult{}, err
	}

	rev, err := s.write(target, data)
	if err != nil {
		return ImportResult{}, err
	}

	result.Session = imported.Info(rev)
	result.Created = true
	result.FrozenNow = freeze && !source.Frozen

	return result, nil
}

// Export writes the stored bytes of a session to path atomically.
// An identical existing file is left untouched; a different one is ErrExists.
func (s *Store) Export(id, path string) (ExportResult, error) {
	data, err := s.raw(id)
	if err != nil {
		return ExportResult{}, err
	}

	sess, err := decode(data)
	if err != nil {
		return ExportResult{}, err
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return ExportResult{}, err
	}

	rev := Digest(data)
	result := ExportResult{Session: sess.Info(rev), Path: abs, SHA256: rev}

	existing, err := os.ReadFile(abs)

	switch {
	case err == nil && Digest(existing) == rev:
		return result, nil
	case err == nil:
		return ExportResult{}, fmt.Errorf("%w: %s has different content", ErrExists, abs)
	case !errors.Is(err, fs.ErrNotExist):
		return ExportResult{}, fmt.Errorf("reading export target: %w", err)
	}

	if err := atomicfile.WriteFile(abs, data); err != nil {
		return ExportResult{}, fmt.Errorf("writing export: %w", err)
	}

	result.Written = true

	return result, nil
}

// Read reads and validates a session file.
func (s *Store) Read(f file.File) (*Session, error) {
	data, err := f.Read()
	if err != nil {
		return nil, fmt.Errorf("reading session: %w", err)
	}

	return decode(data)
}

// Path returns the file holding the session with the given ID.
func (s *Store) Path(id string) file.File {
	return s.dir.WithFile(id + ".json")
}

// patch replaces top-level fields of stored session JSON, preserving all other bytes'
// meaning (including fields this version does not model). A nil value deletes the field.
func patch(data []byte, fields map[string]any) ([]byte, error) {
	var object map[string]json.RawMessage

	if err := json.Unmarshal(data, &object); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidSession, err)
	}

	for key, value := range fields {
		if value == nil {
			delete(object, key)

			continue
		}

		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("encoding %s: %w", key, err)
		}

		object[key] = encoded
	}

	return json.MarshalIndent(object, "", "  ")
}

// sameConversation verifies that child carries the parent's non-system history unchanged.
func sameConversation(parent, child *Session) error {
	encode := func(s *Session) ([]byte, error) {
		var kept []any

		for _, msg := range s.Messages {
			if msg.Role == roles.System {
				continue
			}

			kept = append(kept, msg)
		}

		return json.Marshal(kept)
	}

	want, err := encode(parent)
	if err != nil {
		return err
	}

	got, err := encode(child)
	if err != nil {
		return err
	}

	if string(want) != string(got) {
		return fmt.Errorf("%w: fork of %q does not preserve its history", ErrInvalidSession, parent.ID)
	}

	return nil
}

// sameContent reports whether two sessions hold the same conversation.
func sameContent(a, b *Session) (bool, error) {
	da, err := a.ContentDigest()
	if err != nil {
		return false, err
	}

	db, err := b.ContentDigest()
	if err != nil {
		return false, err
	}

	return da == db, nil
}
