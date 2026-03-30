package filetime_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/idelchi/aura/internal/tools/filetime"
	"github.com/idelchi/aura/pkg/llm/tool"
)

// ── TestRecordReadAndWasRead ───────────────────────────────────────────────────

func TestRecordReadAndWasRead(t *testing.T) {
	t.Parallel()

	tracker := filetime.NewTracker(tool.DefaultReadBeforePolicy())
	path := filepath.Join(t.TempDir(), "some-file.txt")

	if tracker.WasRead(path) {
		t.Fatal("WasRead() = true before any RecordRead, want false")
	}

	tracker.RecordRead(path)

	if !tracker.WasRead(path) {
		t.Errorf("WasRead() = false after RecordRead, want true")
	}
}

func TestRecordReadAndWasRead_UnrecordedPathReturnsFalse(t *testing.T) {
	t.Parallel()

	tracker := filetime.NewTracker(tool.DefaultReadBeforePolicy())
	path := filepath.Join(t.TempDir(), "never-recorded.txt")

	if tracker.WasRead(path) {
		t.Errorf("WasRead(%q) = true for never-recorded path, want false", path)
	}
}

// ── TestAssertRead ─────────────────────────────────────────────────────────────

func TestAssertRead(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		recordIt  bool
		wantErrIs error
	}{
		{
			name:      "unread path returns ErrReadRequired",
			recordIt:  false,
			wantErrIs: filetime.ErrReadRequired,
		},
		{
			name:      "recorded path returns nil",
			recordIt:  true,
			wantErrIs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker := filetime.NewTracker(tool.DefaultReadBeforePolicy())
			path := filepath.Join(t.TempDir(), "target.txt")

			if tt.recordIt {
				tracker.RecordRead(path)
			}

			err := tracker.AssertRead(path)

			if tt.wantErrIs == nil {
				if err != nil {
					t.Errorf("AssertRead() = %v, want nil", err)
				}
			} else {
				if !errors.Is(err, tt.wantErrIs) {
					t.Errorf("AssertRead() error = %v, want wrapping %v", err, tt.wantErrIs)
				}
			}
		})
	}
}

// ── TestClearRead ──────────────────────────────────────────────────────────────

func TestClearRead(t *testing.T) {
	t.Parallel()

	tracker := filetime.NewTracker(tool.DefaultReadBeforePolicy())
	path := filepath.Join(t.TempDir(), "clearme.txt")

	tracker.RecordRead(path)

	if !tracker.WasRead(path) {
		t.Fatal("WasRead() = false after RecordRead, want true")
	}

	tracker.ClearRead(path)

	if tracker.WasRead(path) {
		t.Errorf("WasRead() = true after ClearRead, want false")
	}

	if err := tracker.AssertRead(path); !errors.Is(err, filetime.ErrReadRequired) {
		t.Errorf("AssertRead() after ClearRead = %v, want ErrReadRequired", err)
	}
}

// ── TestNormalization ──────────────────────────────────────────────────────────

func TestNormalization(t *testing.T) {
	t.Parallel()

	tracker := filetime.NewTracker(tool.DefaultReadBeforePolicy())
	path := filepath.Join(t.TempDir(), "norm-file.txt")

	tracker.RecordRead(path)

	if !tracker.WasRead(path) {
		t.Errorf("WasRead(%q) = false after RecordRead with same path, want true", path)
	}

	if err := tracker.AssertRead(path); err != nil {
		t.Errorf("AssertRead(%q) = %v after RecordRead, want nil", path, err)
	}
}

// ── TestIsolation ──────────────────────────────────────────────────────────────

func TestIsolation(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "shared.txt")

	a := filetime.NewTracker(tool.DefaultReadBeforePolicy())
	b := filetime.NewTracker(tool.DefaultReadBeforePolicy())

	a.RecordRead(path)

	if !a.WasRead(path) {
		t.Error("tracker A: WasRead() = false after RecordRead, want true")
	}

	if b.WasRead(path) {
		t.Error("tracker B: WasRead() = true, want false (should be isolated from A)")
	}
}

// ── TestPolicy ─────────────────────────────────────────────────────────────────

func TestPolicy(t *testing.T) {
	t.Parallel()

	custom := tool.ReadBeforePolicy{Write: false, Delete: true}
	tracker := filetime.NewTracker(custom)

	got := tracker.Policy()
	if got != custom {
		t.Errorf("Policy() = %+v, want %+v", got, custom)
	}
}

func TestSetPolicyRecordsRuntimeOverride(t *testing.T) {
	t.Parallel()

	tracker := filetime.NewTracker(tool.DefaultReadBeforePolicy())

	if tracker.RuntimeOverride() != nil {
		t.Fatal("RuntimeOverride() != nil before SetPolicy")
	}

	override := tool.ReadBeforePolicy{Write: false, Delete: false}
	tracker.SetPolicy(override)

	got := tracker.Policy()
	if got != override {
		t.Errorf("Policy() after SetPolicy = %+v, want %+v", got, override)
	}

	if tracker.RuntimeOverride() == nil {
		t.Fatal("RuntimeOverride() = nil after SetPolicy, want non-nil")
	}

	if *tracker.RuntimeOverride() != override {
		t.Errorf("RuntimeOverride() = %+v, want %+v", *tracker.RuntimeOverride(), override)
	}
}

// ── TestNilSafety ──────────────────────────────────────────────────────────────

func TestNilTracker(t *testing.T) {
	t.Parallel()

	var tracker *filetime.Tracker

	// Mutations are no-ops.
	tracker.RecordRead("/some/file")
	tracker.ClearRead("/some/file")
	tracker.SetPolicy(tool.ReadBeforePolicy{})

	// Queries return sane defaults.
	if tracker.WasRead("/some/file") {
		t.Error("nil tracker: WasRead() = true, want false")
	}

	if err := tracker.AssertRead("/some/file"); err != nil {
		t.Errorf("nil tracker: AssertRead() = %v, want nil", err)
	}

	if got := tracker.Policy(); got != tool.DefaultReadBeforePolicy() {
		t.Errorf("nil tracker: Policy() = %+v, want %+v", got, tool.DefaultReadBeforePolicy())
	}

	if tracker.RuntimeOverride() != nil {
		t.Error("nil tracker: RuntimeOverride() != nil, want nil")
	}
}

// ── TestContext ─────────────────────────────────────────────────────────────────

func TestContextRoundTrip(t *testing.T) {
	t.Parallel()

	tracker := filetime.NewTracker(tool.DefaultReadBeforePolicy())
	ctx := filetime.WithTracker(context.Background(), tracker)

	got := filetime.FromContext(ctx)
	if got != tracker {
		t.Error("FromContext did not return the same tracker that was stored")
	}
}

func TestFromContextNil(t *testing.T) {
	t.Parallel()

	got := filetime.FromContext(context.Background())
	if got != nil {
		t.Error("FromContext on bare context should return nil")
	}
}
