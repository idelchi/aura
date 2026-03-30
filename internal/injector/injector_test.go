package injector_test

import (
	"context"
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/injector"
)

// mockInjector is a configurable implementation of injector.Injector.
type mockInjector struct {
	name      string
	timing    injector.Timing
	enabled   bool
	checkFunc func(ctx context.Context, state *injector.State) *injector.Injection
}

func (m *mockInjector) Name() string            { return m.name }
func (m *mockInjector) Timing() injector.Timing { return m.timing }
func (m *mockInjector) Enabled() bool           { return m.enabled }
func (m *mockInjector) Check(ctx context.Context, state *injector.State) *injector.Injection {
	if m.checkFunc == nil {
		return nil
	}

	return m.checkFunc(ctx, state)
}

// mockStatefulInjector extends mockInjector with injector.Stateful.
type mockStatefulInjector struct {
	mockInjector

	fired bool
}

func (m *mockStatefulInjector) HasFired() bool   { return m.fired }
func (m *mockStatefulInjector) MarkFired(v bool) { m.fired = v }

// mockDescriber extends mockInjector with injector.Describer.
type mockDescriber struct {
	mockInjector

	description string
}

func (m *mockDescriber) Describe() string { return m.description }

// newRegistry is a helper that creates a Registry with a nil Debug logger
// (the logger is nil-safe per its implementation).
func newRegistry(t *testing.T) *injector.Registry {
	t.Helper()

	r := injector.New()

	return r
}

// TestRegistry_RegisterAndRun_TimingFilter verifies that Run only triggers
// injectors whose Timing matches the argument.
func TestRegistry_RegisterAndRun_TimingFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		runTiming injector.Timing
		wantFired bool
	}{
		{
			name:      "BeforeChat fires when Run called with BeforeChat",
			runTiming: injector.BeforeChat,
			wantFired: true,
		},
		{
			name:      "AfterResponse does not fire for BeforeChat injector",
			runTiming: injector.AfterResponse,
			wantFired: false,
		},
		{
			name:      "AfterToolExecution does not fire for BeforeChat injector",
			runTiming: injector.AfterToolExecution,
			wantFired: false,
		},
		{
			name:      "OnError does not fire for BeforeChat injector",
			runTiming: injector.OnError,
			wantFired: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := newRegistry(t)

			fired := false

			r.Register(&mockInjector{
				name:    "probe",
				timing:  injector.BeforeChat,
				enabled: true,
				checkFunc: func(_ context.Context, _ *injector.State) *injector.Injection {
					fired = true

					return &injector.Injection{Content: "injected"}
				},
			})

			results := r.Run(context.Background(), tt.runTiming, &injector.State{})

			if fired != tt.wantFired {
				t.Errorf("injector fired = %v, want %v (runTiming=%v)", fired, tt.wantFired, tt.runTiming)
			}

			if tt.wantFired && len(results) == 0 {
				t.Errorf("Run(%v) returned no injections, want 1", tt.runTiming)
			}

			if !tt.wantFired && len(results) != 0 {
				t.Errorf("Run(%v) returned %d injections, want 0", tt.runTiming, len(results))
			}
		})
	}
}

// TestRegistry_Run_SkipsDisabled verifies that disabled injectors are not called.
func TestRegistry_Run_SkipsDisabled(t *testing.T) {
	t.Parallel()

	r := newRegistry(t)

	called := false

	r.Register(&mockInjector{
		name:    "disabled-probe",
		timing:  injector.BeforeChat,
		enabled: false,
		checkFunc: func(_ context.Context, _ *injector.State) *injector.Injection {
			called = true

			return &injector.Injection{Content: "should not appear"}
		},
	})

	results := r.Run(context.Background(), injector.BeforeChat, &injector.State{})

	if called {
		t.Errorf("disabled injector's Check was called, want it skipped")
	}

	if len(results) != 0 {
		t.Errorf("Run returned %d injections for disabled injector, want 0", len(results))
	}
}

// TestRegistry_Run_NilCheckReturnsNoInjection verifies that a nil return from
// Check produces no injection in the results.
func TestRegistry_Run_NilCheckReturnsNoInjection(t *testing.T) {
	t.Parallel()

	r := newRegistry(t)

	r.Register(&mockInjector{
		name:      "nil-check",
		timing:    injector.BeforeChat,
		enabled:   true,
		checkFunc: func(_ context.Context, _ *injector.State) *injector.Injection { return nil },
	})

	results := r.Run(context.Background(), injector.BeforeChat, &injector.State{})
	if len(results) != 0 {
		t.Errorf("Run returned %d injections for nil Check, want 0", len(results))
	}
}

// TestRegistry_Run_AutoPopulatesInjectionName verifies that Run sets
// Injection.Name from the injector's Name() method.
func TestRegistry_Run_AutoPopulatesInjectionName(t *testing.T) {
	t.Parallel()

	r := newRegistry(t)

	r.Register(&mockInjector{
		name:    "my-injector",
		timing:  injector.BeforeChat,
		enabled: true,
		checkFunc: func(_ context.Context, _ *injector.State) *injector.Injection {
			// Return an injection with an empty Name — Run should fill it in.
			return &injector.Injection{Content: "hello"}
		},
	})

	results := r.Run(context.Background(), injector.BeforeChat, &injector.State{})
	if len(results) != 1 {
		t.Fatalf("Run returned %d injections, want 1", len(results))
	}

	if results[0].Name != "my-injector" {
		t.Errorf("Injection.Name = %q, want %q", results[0].Name, "my-injector")
	}
}

// TestRegistry_Run_MultipleInjectorsSameTiming verifies that all enabled
// injectors at the same timing are executed and collected.
func TestRegistry_Run_MultipleInjectorsSameTiming(t *testing.T) {
	t.Parallel()

	r := newRegistry(t)

	for _, name := range []string{"alpha", "beta", "gamma"} {
		n := name // captured for closure
		r.Register(&mockInjector{
			name:    n,
			timing:  injector.BeforeChat,
			enabled: true,
			checkFunc: func(_ context.Context, _ *injector.State) *injector.Injection {
				return &injector.Injection{Content: n + "-content"}
			},
		})
	}

	results := r.Run(context.Background(), injector.BeforeChat, &injector.State{})
	if len(results) != 3 {
		t.Fatalf("Run returned %d injections, want 3", len(results))
	}

	names := make(map[string]bool)

	for _, inj := range results {
		names[inj.Name] = true
	}

	for _, want := range []string{"alpha", "beta", "gamma"} {
		if !names[want] {
			t.Errorf("injection %q not found in results", want)
		}
	}
}

// TestRegistry_FiredState verifies that only Stateful injectors where HasFired()==true
// appear in the returned map.
func TestRegistry_FiredState(t *testing.T) {
	t.Parallel()

	r := newRegistry(t)

	firedOne := &mockStatefulInjector{
		mockInjector: mockInjector{name: "fired-one", timing: injector.BeforeChat, enabled: true},
		fired:        true,
	}
	notFired := &mockStatefulInjector{
		mockInjector: mockInjector{name: "not-fired", timing: injector.BeforeChat, enabled: true},
		fired:        false,
	}
	nonStateful := &mockInjector{
		name:    "non-stateful",
		timing:  injector.BeforeChat,
		enabled: true,
	}

	r.Register(firedOne)
	r.Register(notFired)
	r.Register(nonStateful)

	state := r.FiredState()

	if !state["fired-one"] {
		t.Errorf("FiredState missing %q, want true", "fired-one")
	}

	if _, ok := state["not-fired"]; ok {
		t.Errorf("FiredState contains %q, want absent", "not-fired")
	}

	if _, ok := state["non-stateful"]; ok {
		t.Errorf("FiredState contains non-stateful injector %q, want absent", "non-stateful")
	}
}

// TestRegistry_RestoreFiredState verifies that RestoreFiredState marks the
// matching Stateful injectors as fired.
func TestRegistry_RestoreFiredState(t *testing.T) {
	t.Parallel()

	r := newRegistry(t)

	a := &mockStatefulInjector{
		mockInjector: mockInjector{name: "a", timing: injector.AfterResponse, enabled: true},
	}
	b := &mockStatefulInjector{
		mockInjector: mockInjector{name: "b", timing: injector.AfterResponse, enabled: true},
	}

	r.Register(a)
	r.Register(b)

	r.RestoreFiredState(map[string]bool{"a": true})

	if !a.HasFired() {
		t.Errorf("injector %q HasFired() = false after restore, want true", "a")
	}

	if b.HasFired() {
		t.Errorf("injector %q HasFired() = true after restore, want false (not in snapshot)", "b")
	}
}

// TestRegistry_FiredState_RestoreRoundTrip verifies that saving FiredState and
// restoring it into a fresh registry correctly replicates the fired set.
func TestRegistry_FiredState_RestoreRoundTrip(t *testing.T) {
	t.Parallel()

	// Build original registry with some injectors fired.
	orig := newRegistry(t)

	origA := &mockStatefulInjector{
		mockInjector: mockInjector{name: "injA", timing: injector.BeforeChat, enabled: true},
		fired:        true,
	}
	origB := &mockStatefulInjector{
		mockInjector: mockInjector{name: "injB", timing: injector.BeforeChat, enabled: true},
		fired:        false,
	}

	orig.Register(origA)
	orig.Register(origB)

	snapshot := orig.FiredState()

	// Build a fresh registry with the same injectors (all unfired).
	fresh := newRegistry(t)

	freshA := &mockStatefulInjector{
		mockInjector: mockInjector{name: "injA", timing: injector.BeforeChat, enabled: true},
	}
	freshB := &mockStatefulInjector{
		mockInjector: mockInjector{name: "injB", timing: injector.BeforeChat, enabled: true},
	}

	fresh.Register(freshA)
	fresh.Register(freshB)

	fresh.RestoreFiredState(snapshot)

	if !freshA.HasFired() {
		t.Errorf("round-trip: injA HasFired() = false, want true")
	}

	if freshB.HasFired() {
		t.Errorf("round-trip: injB HasFired() = true, want false")
	}
}

// TestRegistry_Display_NonEmpty verifies that Display produces a non-empty
// string containing timing headers for registered injectors.
func TestRegistry_Display_NonEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		timing     injector.Timing
		wantHeader string
		wantName   string
		wantStatus string
	}{
		{
			name:       "enabled BeforeChat injector appears under BeforeChat header",
			timing:     injector.BeforeChat,
			wantHeader: "BeforeChat:",
			wantName:   "my-probe",
			wantStatus: "enabled",
		},
		{
			name:       "disabled AfterResponse injector shows disabled status",
			timing:     injector.AfterResponse,
			wantHeader: "AfterResponse:",
			wantName:   "slow-probe",
			wantStatus: "disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := newRegistry(t)
			r.Register(&mockInjector{
				name:    tt.wantName,
				timing:  tt.timing,
				enabled: tt.wantStatus == "enabled",
			})

			got := r.Display()

			if got == "" {
				t.Fatalf("Display() returned empty string, want non-empty")
			}

			if !strings.Contains(got, tt.wantHeader) {
				t.Errorf("Display() missing header %q\ngot:\n%s", tt.wantHeader, got)
			}

			if !strings.Contains(got, tt.wantName) {
				t.Errorf("Display() missing injector name %q\ngot:\n%s", tt.wantName, got)
			}

			if !strings.Contains(got, tt.wantStatus) {
				t.Errorf("Display() missing status %q\ngot:\n%s", tt.wantStatus, got)
			}
		})
	}
}

// TestRegistry_Display_DescriberIncluded verifies that Describer output appears
// in Display() for injectors that implement the interface.
func TestRegistry_Display_DescriberIncluded(t *testing.T) {
	t.Parallel()

	r := newRegistry(t)
	r.Register(&mockDescriber{
		mockInjector: mockInjector{
			name:    "described",
			timing:  injector.OnError,
			enabled: true,
		},
		description: "extra-detail",
	})

	got := r.Display()

	if !strings.Contains(got, "extra-detail") {
		t.Errorf("Display() missing Describer output %q\ngot:\n%s", "extra-detail", got)
	}
}

// TestRegistry_Display_EmptyRegistryReturnsEmpty verifies that an empty
// registry (no injectors registered) produces an empty Display string.
func TestRegistry_Display_EmptyRegistryReturnsEmpty(t *testing.T) {
	t.Parallel()

	r := newRegistry(t)
	got := r.Display()

	if got != "" {
		t.Errorf("Display() on empty registry = %q, want empty string", got)
	}
}

// --- Typed injection tests ---

// mockBeforeChatChecker implements BeforeChatChecker for typed Run tests.
type mockBeforeChatChecker struct {
	mockInjector

	result *injector.BeforeChatInjection
}

func (m *mockBeforeChatChecker) CheckBeforeChat(_ context.Context, _ *injector.State) *injector.BeforeChatInjection {
	return m.result
}

// mockAfterResponseChecker implements AfterResponseChecker.
type mockAfterResponseChecker struct {
	mockInjector

	result *injector.AfterResponseInjection
}

func (m *mockAfterResponseChecker) CheckAfterResponse(
	_ context.Context,
	_ *injector.State,
) *injector.AfterResponseInjection {
	return m.result
}

// mockAfterToolChecker implements AfterToolChecker.
type mockAfterToolChecker struct {
	mockInjector

	result *injector.AfterToolInjection
}

func (m *mockAfterToolChecker) CheckAfterTool(_ context.Context, _ *injector.State) *injector.AfterToolInjection {
	return m.result
}

// mockOnErrorChecker implements OnErrorChecker.
type mockOnErrorChecker struct {
	mockInjector

	result *injector.OnErrorInjection
}

func (m *mockOnErrorChecker) CheckOnError(_ context.Context, _ *injector.State) *injector.OnErrorInjection {
	return m.result
}

// mockBeforeCompactionChecker implements BeforeCompactionChecker.
type mockBeforeCompactionChecker struct {
	mockInjector

	result *injector.BeforeCompactionInjection
}

func (m *mockBeforeCompactionChecker) CheckBeforeCompaction(
	_ context.Context,
	_ *injector.State,
) *injector.BeforeCompactionInjection {
	return m.result
}

func TestBases_ConvertsTypedSlice(t *testing.T) {
	t.Parallel()

	items := []injector.BeforeChatInjection{
		{Injection: injector.Injection{Name: "a", Content: "hello"}},
		{Injection: injector.Injection{Name: "b", Content: "world"}},
	}

	bases := injector.Bases(items)
	if len(bases) != 2 {
		t.Fatalf("Bases() returned %d items, want 2", len(bases))
	}

	if bases[0].Name != "a" || bases[0].Content != "hello" {
		t.Errorf("bases[0] = %+v, want Name=a Content=hello", bases[0])
	}

	if bases[1].Name != "b" || bases[1].Content != "world" {
		t.Errorf("bases[1] = %+v, want Name=b Content=world", bases[1])
	}
}

func TestBases_EmptySlice(t *testing.T) {
	t.Parallel()

	bases := injector.Bases([]injector.AfterToolInjection{})
	if bases != nil {
		t.Errorf("Bases(empty) = %v, want nil", bases)
	}
}

func TestRegistry_RunBeforeChat(t *testing.T) {
	t.Parallel()

	r := newRegistry(t)
	r.Register(&mockBeforeChatChecker{
		mockInjector: mockInjector{name: "req-mod", timing: injector.BeforeChat, enabled: true},
		result: &injector.BeforeChatInjection{
			Injection: injector.Injection{Content: "before-chat"},
		},
	})

	results := r.RunBeforeChat(context.Background(), &injector.State{})
	if len(results) != 1 {
		t.Fatalf("RunBeforeChat returned %d results, want 1", len(results))
	}

	if results[0].Content != "before-chat" {
		t.Errorf("Content = %q, want %q", results[0].Content, "before-chat")
	}
}

func TestRegistry_RunAfterResponse(t *testing.T) {
	t.Parallel()

	r := newRegistry(t)
	r.Register(&mockAfterResponseChecker{
		mockInjector: mockInjector{name: "resp-mod", timing: injector.AfterResponse, enabled: true},
		result: &injector.AfterResponseInjection{
			Injection: injector.Injection{Content: "after-response"},
		},
	})

	results := r.RunAfterResponse(context.Background(), &injector.State{})
	if len(results) != 1 {
		t.Fatalf("RunAfterResponse returned %d results, want 1", len(results))
	}

	if results[0].Content != "after-response" {
		t.Errorf("Content = %q, want %q", results[0].Content, "after-response")
	}
}

func TestRegistry_RunAfterTool(t *testing.T) {
	t.Parallel()

	output := "modified-output"

	r := newRegistry(t)
	r.Register(&mockAfterToolChecker{
		mockInjector: mockInjector{name: "tool-mod", timing: injector.AfterToolExecution, enabled: true},
		result: &injector.AfterToolInjection{
			Injection: injector.Injection{Content: "after-tool"},
			Output:    &output,
		},
	})

	results := r.RunAfterTool(context.Background(), &injector.State{})
	if len(results) != 1 {
		t.Fatalf("RunAfterTool returned %d results, want 1", len(results))
	}

	if results[0].Output == nil || *results[0].Output != "modified-output" {
		t.Errorf("Output = %v, want %q", results[0].Output, "modified-output")
	}
}

func TestRegistry_RunOnError(t *testing.T) {
	t.Parallel()

	r := newRegistry(t)
	r.Register(&mockOnErrorChecker{
		mockInjector: mockInjector{name: "err-mod", timing: injector.OnError, enabled: true},
		result: &injector.OnErrorInjection{
			Injection: injector.Injection{Content: "error-handler"},
		},
	})

	results := r.RunOnError(context.Background(), &injector.State{})
	if len(results) != 1 {
		t.Fatalf("RunOnError returned %d results, want 1", len(results))
	}

	if results[0].Content != "error-handler" {
		t.Errorf("Content = %q, want %q", results[0].Content, "error-handler")
	}
}

func TestRegistry_RunBeforeCompaction(t *testing.T) {
	t.Parallel()

	r := newRegistry(t)
	r.Register(&mockBeforeCompactionChecker{
		mockInjector: mockInjector{name: "compact-mod", timing: injector.BeforeCompaction, enabled: true},
		result: &injector.BeforeCompactionInjection{
			Injection: injector.Injection{Content: "before-compact"},
		},
	})

	results := r.RunBeforeCompaction(context.Background(), &injector.State{})
	if len(results) != 1 {
		t.Fatalf("RunBeforeCompaction returned %d results, want 1", len(results))
	}

	if results[0].Content != "before-compact" {
		t.Errorf("Content = %q, want %q", results[0].Content, "before-compact")
	}
}

func TestRegistry_RunTyped_SkipsDisabled(t *testing.T) {
	t.Parallel()

	r := newRegistry(t)
	r.Register(&mockBeforeChatChecker{
		mockInjector: mockInjector{name: "disabled", timing: injector.BeforeChat, enabled: false},
		result:       &injector.BeforeChatInjection{Injection: injector.Injection{Content: "should-not-appear"}},
	})

	results := r.RunBeforeChat(context.Background(), &injector.State{})
	if len(results) != 0 {
		t.Errorf("RunBeforeChat returned %d results for disabled injector, want 0", len(results))
	}
}

func TestRegistry_RunTyped_SkipsNonChecker(t *testing.T) {
	t.Parallel()

	// Register a plain Injector (not a BeforeChatChecker) at BeforeChat timing.
	r := newRegistry(t)
	r.Register(&mockInjector{
		name:    "plain",
		timing:  injector.BeforeChat,
		enabled: true,
		checkFunc: func(_ context.Context, _ *injector.State) *injector.Injection {
			return &injector.Injection{Content: "should-not-appear"}
		},
	})

	results := r.RunBeforeChat(context.Background(), &injector.State{})
	if len(results) != 0 {
		t.Errorf("RunBeforeChat returned %d results for non-checker, want 0", len(results))
	}
}

func TestRegistry_RunTyped_NilResult(t *testing.T) {
	t.Parallel()

	r := newRegistry(t)
	r.Register(&mockBeforeChatChecker{
		mockInjector: mockInjector{name: "nil-result", timing: injector.BeforeChat, enabled: true},
		result:       nil, // CheckBeforeChat returns nil
	})

	results := r.RunBeforeChat(context.Background(), &injector.State{})
	if len(results) != 0 {
		t.Errorf("RunBeforeChat returned %d results for nil check, want 0", len(results))
	}
}
