package assistant

import (
	"context"
	"testing"
	"time"

	"github.com/idelchi/aura/internal/slash/commands"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/usage"
)

func TestExitInterruptsActiveProviderAndFinishesLoop(t *testing.T) {
	a := sessionAssistant(t, t.TempDir())
	recordEvents(t, a)
	a.slashRegistry.Register(commands.Exit())
	started := make(chan struct{})
	a.agent.Provider = contextProbe{
		compactionRequestProbe: compactionRequestProbe{Provider: a.agent.Provider},
		chat: func(ctx context.Context, _ request.Request) (message.Message, usage.Usage, error) {
			close(started)
			<-ctx.Done()
			return message.Message{}, usage.Usage{}, ctx.Err()
		},
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	inputs := make(chan ui.UserInput, 2)
	done := make(chan error, 1)
	go func() { done <- a.Loop(ctx, inputs, nil, nil) }()
	inputs <- ui.UserInput{Kind: ui.InputPrompt, Text: "start a long response"}
	select {
	case <-started:
	case <-ctx.Done():
		<-done
		t.Fatal("provider was not started")
	}
	inputs <- ui.UserInput{Kind: ui.InputCommand, Text: "/exit"}
	if err := <-done; err != nil {
		t.Fatalf("/exit did not stop normally before the session deadline: %v", err)
	}
	if ctx.Err() != nil {
		t.Fatal("/exit waited for external session cancellation")
	}
}
