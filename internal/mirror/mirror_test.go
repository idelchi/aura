package mirror

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestTranscriptAndTerminalOutputShareLog(t *testing.T) {
	terminal, err := os.CreateTemp(t.TempDir(), "terminal")
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = terminal, terminal
	defer func() { os.Stdout, os.Stderr = stdout, stderr }()

	path := filepath.Join(t.TempDir(), "transcript.log")
	m, err := New(path)
	if err != nil {
		t.Fatal(err)
	}

	// Direct transcript writes must not redraw the terminal. Concurrent diagnostic
	// writes still reach both destinations, and each transcript record stays intact.
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			if _, err := fmt.Fprintf(m, "\x1b[36mrecord-%02d\x1b[0m\n", i); err != nil {
				t.Error(err)
			}
		})
	}
	fmt.Fprintln(os.Stdout, "stdout diagnostic")
	fmt.Fprintln(os.Stderr, "stderr diagnostic")
	wg.Wait()
	m.Close()

	logged, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 20 {
		if strings.Count(string(logged), fmt.Sprintf("record-%02d\n", i)) != 1 {
			t.Fatalf("record %d was lost, split, or repeated: %q", i, logged)
		}
	}
	if strings.Contains(string(logged), "\x1b") {
		t.Fatal("transcript contains terminal escapes")
	}
	for _, want := range []string{"stdout diagnostic", "stderr diagnostic"} {
		if !strings.Contains(string(logged), want) {
			t.Fatalf("missing %s", want)
		}
	}
	terminal.Seek(0, io.SeekStart)
	screen, err := io.ReadAll(terminal)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(screen), "record-") || !strings.Contains(string(screen), "stdout diagnostic") {
		t.Fatalf("semantic transcript leaked to terminal or diagnostics disappeared: %q", screen)
	}
}
