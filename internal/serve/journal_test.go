package serve

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/idelchi/aura/pkg/automation"
)

func openTestJournal(t *testing.T, path string) (*Journal, []automation.Event, bool) {
	t.Helper()

	j, events, repaired, err := OpenJournal(path, "sess")
	if err != nil {
		t.Fatalf("OpenJournal: %v", err)
	}

	t.Cleanup(func() { j.Close() })

	return j, events, repaired
}

func TestJournalSequencesAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal", "sess.jsonl")

	j, _, _ := openTestJournal(t, path)

	for i := range 3 {
		if e, err := j.Append(automation.EventNotice, "", automation.Notice{Message: "n"}); err != nil || e.Seq != int64(i+1) {
			t.Fatalf("Append %d = %+v, %v", i, e, err)
		}
	}

	j.Close()

	again, events, repaired := openTestJournal(t, path)
	if len(events) != 3 || repaired || again.LastSeq() != 3 {
		t.Fatalf("reopened with %d events, repaired=%v, last=%d", len(events), repaired, again.LastSeq())
	}

	if e, err := again.Append(automation.EventRunAccepted, "run_x", automation.RunData{}); err != nil || e.Seq != 4 || e.RunID != "run_x" {
		t.Fatalf("Append after reopen = %+v, %v", e, err)
	}
}

func TestJournalTruncatesTornFinalLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sess.jsonl")

	j, _, _ := openTestJournal(t, path)
	j.Append(automation.EventNotice, "", automation.Notice{Message: "kept"})
	j.Close()

	// A crash mid-write leaves an unterminated, partial final line.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}

	f.WriteString(`{"seq":2,"time":"2026-10-26T00:00:00Z","type":"torn-wri`)
	f.Close()

	again, events, repaired := openTestJournal(t, path)
	if !repaired || len(events) != 1 || again.LastSeq() != 1 {
		t.Fatalf("repaired=%v events=%d last=%d", repaired, len(events), again.LastSeq())
	}

	if e, err := again.Append(automation.EventNotice, "", automation.Notice{Message: "next"}); err != nil || e.Seq != 2 {
		t.Fatalf("Append after repair = %+v, %v", e, err)
	}

	data, _ := os.ReadFile(path)
	if strings.Count(string(data), "\n") != 2 || strings.Contains(string(data), "torn-wri") {
		t.Fatalf("journal after repair:\n%s", data)
	}
}

func TestJournalRejectsCorruptionBeforeTheTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sess.jsonl")

	j, _, _ := openTestJournal(t, path)
	j.Append(automation.EventNotice, "", automation.Notice{Message: "a"})
	j.Append(automation.EventNotice, "", automation.Notice{Message: "b"})
	j.Close()

	data, _ := os.ReadFile(path)
	lines := strings.SplitN(string(data), "\n", 2)
	os.WriteFile(path, []byte("garbage\n"+lines[1]), 0o600)

	if _, _, _, err := OpenJournal(path, "sess"); !errors.Is(err, ErrJournalCorrupt) {
		t.Fatalf("OpenJournal = %v, want ErrJournalCorrupt", err)
	}

	os.WriteFile(path, data, 0o600)

	if _, _, _, err := OpenJournal(path, "other"); !errors.Is(err, ErrJournalCorrupt) {
		t.Fatalf("journal of another session opened: %v", err)
	}
}

func TestJournalReadBoundsAndPaging(t *testing.T) {
	j, _, _ := openTestJournal(t, filepath.Join(t.TempDir(), "sess.jsonl"))

	for range 10 {
		j.Append(automation.EventNotice, "", automation.Notice{Message: "x"})
	}

	events, last, _, err := j.Read(3, 4)
	if err != nil || last != 10 || len(events) != 4 || events[0].Seq != 4 || events[3].Seq != 7 {
		t.Fatalf("Read(3,4) = %d events from %v, last %d, %v", len(events), seqs(events), last, err)
	}

	if events, _, _, err := j.Read(10, 100); err != nil || len(events) != 0 {
		t.Fatalf("Read(at end) = %v, %v", seqs(events), err)
	}

	for _, after := range []int64{-1, 11} {
		_, _, _, err := j.Read(after, 1)

		var e *automation.Error
		if !errors.As(err, &e) || e.Code != automation.CodeCursorOutOfRange || e.Details.LastSeq == nil || *e.Details.LastSeq != 10 {
			t.Fatalf("Read(%d) = %v, want cursor_out_of_range with last_seq", after, err)
		}
	}
}

func TestJournalReadersNeverMissAppends(t *testing.T) {
	j, _, _ := openTestJournal(t, filepath.Join(t.TempDir(), "sess.jsonl"))

	const total = 2000

	done := make(chan []int64)

	// A deliberately slow reader still sees every event, in order.
	go func() {
		var got []int64

		after := int64(0)

		for len(got) < total {
			events, _, changed, err := j.Read(after, 7)
			if err != nil {
				t.Error(err)
				break
			}

			for _, e := range events {
				got = append(got, e.Seq)
				after = e.Seq
			}

			if len(events) == 0 {
				select {
				case <-changed:
				case <-time.After(10 * time.Second):
					t.Error("reader starved")
					done <- got

					return
				}
			}

			time.Sleep(time.Microsecond)
		}

		done <- got
	}()

	for range total {
		if _, err := j.Append(automation.EventNotice, "", automation.Notice{Message: strings.Repeat("y", 100)}); err != nil {
			t.Fatal(err)
		}
	}

	got := <-done
	if len(got) != total {
		t.Fatalf("reader saw %d of %d events", len(got), total)
	}

	for i, seq := range got {
		if seq != int64(i+1) {
			t.Fatalf("event %d has seq %d", i, seq)
		}
	}
}

func seqs(events []automation.Event) []int64 {
	out := make([]int64, len(events))
	for i, e := range events {
		out[i] = e.Seq
	}

	return out
}
