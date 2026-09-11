package assistant

import (
	"errors"
	"testing"

	"github.com/idelchi/aura/internal/injector"
)

func TestInjectionStop(t *testing.T) {
	if err := injectionStop([]injector.Injection{{Content: "advice"}}); err != nil {
		t.Fatal(err)
	}
	err := injectionStop([]injector.Injection{{Name: "contract", Stop: "invalid response"}})
	if !errors.Is(err, ErrPolicyStopped) {
		t.Fatalf("expected explicit stop, got %v", err)
	}
}

// A terminal answer need not validate or invoke any compaction machinery.
func TestFinalAnswerDoesNotCompact(t *testing.T) {
	a := selectionAssistant(t)
	a.cfg.Features.Compaction.MaxTokens = 1
	a.cfg.Features.Compaction.Agent = "does-not-exist"
	a.finalizeTurn()
	if got := a.SessionStats().Snapshot().Compactions; got != 0 {
		t.Fatalf("unexpected compactions: %d", got)
	}
}

// TestProviderOverheadRaisesEstimate keeps calibration conservative and resettable.
func TestProviderOverheadRaisesEstimate(t *testing.T) {
	a := selectionAssistant(t)
	baseline := a.EstimateTokens(t.Context())
	a.tokens.overhead = 37
	if got := a.EstimateTokens(t.Context()); got != baseline+37 {
		t.Fatalf("provider overhead missing: %d", got)
	}
	a.ResetTokens()
	if a.tokens.overhead != 0 {
		t.Fatal("token reset retained provider-specific calibration")
	}
}
