package serve

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/idelchi/aura/pkg/automation"
)

// ErrJournalCorrupt is returned when a journal line other than a torn final line is invalid.
var ErrJournalCorrupt = errors.New("journal corrupt")

// Journal is the durable, sequenced event log of one session.
//
// Each event is one JSON line appended with a single write. Lifecycle events are fsynced.
// Sequence numbers start at 1, are contiguous, and continue across server restarts. A
// crash can only leave a torn final line, which Open truncates; such a line was never
// served because readers only see lines below the committed end.
//
// The journal has a single writer: the process holding the session's lease.
type Journal struct {
	mu      sync.Mutex
	file    *os.File
	session string
	offsets []int64       // offsets[i] is where the event with seq i+1 starts
	end     int64         // committed end offset
	changed chan struct{} // closed and replaced after every append
}

// OpenJournal opens or creates the journal at path, validating it and truncating a torn
// final line. It returns the journal, its existing events and whether a tail was repaired.
func OpenJournal(path, sessionID string) (*Journal, []automation.Event, bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, nil, false, fmt.Errorf("creating journal directory: %w", err)
	}

	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, nil, false, fmt.Errorf("opening journal: %w", err)
	}

	data, err := io.ReadAll(file)
	if err != nil {
		file.Close()

		return nil, nil, false, fmt.Errorf("reading journal: %w", err)
	}

	j := &Journal{file: file, session: sessionID, changed: make(chan struct{})}

	var (
		events   []automation.Event
		offset   int64
		repaired bool
	)

	for len(data) > 0 {
		newline := bytes.IndexByte(data, '\n')

		var event automation.Event

		valid := newline >= 0 && json.Unmarshal(data[:newline], &event) == nil

		if !valid {
			if newline >= 0 && bytes.IndexByte(data[newline+1:], '\n') >= 0 {
				file.Close()

				return nil, nil, false, fmt.Errorf("%w: %s line %d", ErrJournalCorrupt, path, len(events)+1)
			}

			// A torn final line: the write never completed, so the event never happened.
			if err := file.Truncate(offset); err != nil {
				file.Close()

				return nil, nil, false, fmt.Errorf("truncating torn journal tail: %w", err)
			}

			repaired = true

			break
		}

		if event.Seq != int64(len(events))+1 || event.SessionID != sessionID {
			file.Close()

			return nil, nil, false, fmt.Errorf("%w: %s line %d has seq %d of session %q",
				ErrJournalCorrupt, path, len(events)+1, event.Seq, event.SessionID)
		}

		j.offsets = append(j.offsets, offset)
		events = append(events, event)

		offset += int64(newline + 1)
		data = data[newline+1:]
	}

	j.end = offset

	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		file.Close()

		return nil, nil, false, fmt.Errorf("positioning journal: %w", err)
	}

	return j, events, repaired, nil
}

// LastSeq returns the sequence number of the last event (0 when empty).
func (j *Journal) LastSeq() int64 {
	j.mu.Lock()
	defer j.mu.Unlock()

	return int64(len(j.offsets))
}

// Append writes one event and returns it. Lifecycle events are fsynced before returning.
func (j *Journal) Append(typ automation.EventType, runID string, data any) (automation.Event, error) {
	encoded, err := json.Marshal(data)
	if err != nil {
		return automation.Event{}, fmt.Errorf("encoding %s: %w", typ, err)
	}

	j.mu.Lock()
	defer j.mu.Unlock()

	event := automation.Event{
		Seq:       int64(len(j.offsets)) + 1,
		Time:      time.Now().UTC(),
		Type:      typ,
		SessionID: j.session,
		RunID:     automation.NullableString(runID),
		Data:      encoded,
	}

	line, err := json.Marshal(event)
	if err != nil {
		return automation.Event{}, fmt.Errorf("encoding %s: %w", typ, err)
	}

	line = append(line, '\n')

	if _, err := j.file.WriteAt(line, j.end); err != nil {
		// Drop whatever part of the line reached the file; the event did not happen.
		j.file.Truncate(j.end)

		return automation.Event{}, fmt.Errorf("writing journal: %w", err)
	}

	if typ.IsLifecycle() {
		if err := j.file.Sync(); err != nil {
			j.file.Truncate(j.end)

			return automation.Event{}, fmt.Errorf("syncing journal: %w", err)
		}
	}

	j.offsets = append(j.offsets, j.end)
	j.end += int64(len(line))

	close(j.changed)
	j.changed = make(chan struct{})

	return event, nil
}

// Read returns up to limit events with seq > after, the last seq, and a channel that is
// closed by the next append (so a reader that found nothing new can wait without a race).
// After must satisfy 0 <= after <= last seq.
func (j *Journal) Read(after int64, limit int) ([]automation.Event, int64, <-chan struct{}, error) {
	j.mu.Lock()

	last := int64(len(j.offsets))
	changed := j.changed
	end := j.end

	if after < 0 || after > last {
		j.mu.Unlock()

		lastSeq := last
		e := automation.NewError(automation.CodeCursorOutOfRange, "cursor %d is outside the journal (last seq %d)", after, last)
		e.Details.LastSeq = &lastSeq

		return nil, last, changed, e
	}

	if after == last || limit <= 0 {
		j.mu.Unlock()

		return nil, last, changed, nil
	}

	start := j.offsets[after]

	stop := end
	if int(after)+limit < len(j.offsets) {
		stop = j.offsets[int(after)+limit]
	}

	j.mu.Unlock()

	buf := make([]byte, stop-start)

	if _, err := j.file.ReadAt(buf, start); err != nil {
		return nil, last, changed, fmt.Errorf("reading journal: %w", err)
	}

	var events []automation.Event

	for line := range bytes.SplitSeq(bytes.TrimSuffix(buf, []byte("\n")), []byte("\n")) {
		var event automation.Event

		if err := json.Unmarshal(line, &event); err != nil {
			return nil, last, changed, fmt.Errorf("%w: %w", ErrJournalCorrupt, err)
		}

		events = append(events, event)
	}

	return events, last, changed, nil
}

// Close syncs and closes the journal file.
func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()

	if err := j.file.Sync(); err != nil {
		j.file.Close()

		return err
	}

	return j.file.Close()
}
