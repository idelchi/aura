package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// ListResult holds session summaries and any warnings encountered during listing.
type ListResult struct {
	Summaries []Summary
	Warnings  []string
}

// ErrNotFound is returned by Find when no session matches the given prefix.
var ErrNotFound = errors.New("session not found")

// Store handles JSON file-based session persistence.
type Store struct {
	dir folder.Folder
}

// NewStore creates a Store rooted at the given directory.
func NewStore(dir string) *Store {
	return &Store{dir: folder.New(dir)}
}

// Save writes a session to disk as JSON.
func (s *Store) Save(session *Session) error {
	if err := s.dir.Create(); err != nil {
		return fmt.Errorf("creating session directory: %w", err)
	}

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling session: %w", err)
	}

	if err := s.Path(session.ID).Write(data); err != nil {
		return fmt.Errorf("writing session: %w", err)
	}

	return nil
}

// Load reads a session from disk by ID.
func (s *Store) Load(id string) (*Session, error) {
	return s.Read(s.Path(id))
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

	summaries := make([]Summary, 0, len(listing))

	var warnings []string

	for _, f := range listing {
		if f.Extension() != "json" {
			continue
		}

		session, err := s.Read(f)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("skipping corrupt session %s: %v", f.Base(), err))

			continue
		}

		summaries = append(summaries, Summary{
			ID:               session.ID,
			Title:            session.Title,
			UpdatedAt:        session.UpdatedAt,
			FirstUserMessage: session.FirstUserContent(),
		})
	}

	slices.SortFunc(summaries, func(a, b Summary) int {
		switch {
		case a.UpdatedAt.After(b.UpdatedAt):
			return -1
		case a.UpdatedAt.Before(b.UpdatedAt):
			return 1
		default:
			return 0
		}
	})

	return ListResult{Summaries: summaries, Warnings: warnings}, nil
}

// Find resolves a session ID prefix to a full session ID.
// Returns an error if zero or multiple sessions match.
func (s *Store) Find(prefix string) (string, error) {
	result, err := s.List()
	if err != nil {
		return "", err
	}

	var matches []string

	for _, sm := range result.Summaries {
		if len(sm.ID) >= len(prefix) && sm.ID[:len(prefix)] == prefix {
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

// Rename changes a session's file name on disk from oldID to newID.
func (s *Store) Rename(oldID, newID string) error {
	return s.Path(oldID).Rename(s.Path(newID))
}

func (s *Store) Read(f file.File) (*Session, error) {
	data, err := f.Read()
	if err != nil {
		return nil, fmt.Errorf("reading session: %w", err)
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("unmarshaling session: %w", err)
	}

	return &session, nil
}

func (s *Store) Path(id string) file.File {
	return s.dir.WithFile(id + ".json")
}
