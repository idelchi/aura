package serve

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/part"
	"github.com/idelchi/aura/pkg/llm/roles"
)

func TestOperatorTranscriptDoesNotNeedThePublicJournal(t *testing.T) {
	u := NewUI()
	u.server = &Server{} // No journal: recording restored history must not write public events.
	var log bytes.Buffer
	u.RecordTo(&log)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	finished := make(chan error, 1)
	go func() { finished <- u.Run(ctx) }()
	u.Events() <- ui.SessionRestored{Messages: message.Messages{{Role: roles.Assistant,
		Parts: []part.Part{{Type: part.Thinking, Text: "operator-only reasoning"}, {Type: part.Content, Text: "Public answer"}},
	}}}
	flushed := make(chan struct{})
	u.Events() <- ui.Flush{Done: flushed}
	<-flushed
	cancel()
	<-finished
	if !strings.Contains(log.String(), "operator-only reasoning") || !strings.Contains(log.String(), "Public answer") {
		t.Fatal(log.String())
	}
}

type brokenTranscript struct{}

func (brokenTranscript) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestTranscriptFailureStopsTheAdapter(t *testing.T) {
	u := NewUI()
	u.server = &Server{}
	u.RecordTo(brokenTranscript{})
	u.Events() <- ui.SessionRestored{Messages: message.Messages{{Role: roles.User, Content: "assignment"}}}
	if err := u.Run(t.Context()); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
}
