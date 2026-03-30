package call_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/idelchi/aura/pkg/llm/tool/call"
)

func TestSetArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args map[string]any
	}{
		{
			name: "single string argument",
			args: map[string]any{"file": "main.go"},
		},
		{
			name: "multiple arguments",
			args: map[string]any{"cmd": "ls", "dir": "/tmp"},
		},
		{
			name: "nil arguments",
			args: nil,
		},
		{
			name: "empty arguments",
			args: map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := &call.Call{}
			c.SetArgs(tt.args)

			// Arguments field must be set to the provided map.
			if len(c.Arguments) != len(tt.args) {
				t.Errorf("Arguments length = %d, want %d", len(c.Arguments), len(tt.args))
			}

			for k, want := range tt.args {
				if got := c.Arguments[k]; got != want {
					t.Errorf("Arguments[%q] = %v, want %v", k, got, want)
				}
			}

			// ArgumentsDisplay must be set (may be truncated) when args are non-empty.
			if len(tt.args) > 0 && c.ArgumentsDisplay == "" {
				t.Error("ArgumentsDisplay is empty, want non-empty for non-nil args")
			}

			// ArgumentsDisplay must not exceed MaxArgsLen + len(" [...truncated]").
			const maxDisplay = call.MaxArgsLen
			if len(c.ArgumentsDisplay) > maxDisplay {
				t.Errorf("ArgumentsDisplay length = %d, exceeds max %d", len(c.ArgumentsDisplay), maxDisplay)
			}
		})
	}
}

func TestComplete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		result       string
		wantState    call.State
		wantTruncate bool // result longer than MaxResultLen
	}{
		{
			name:      "short result",
			result:    "done",
			wantState: call.Complete,
		},
		{
			name:      "empty result",
			result:    "",
			wantState: call.Complete,
		},
		{
			name:         "long result gets truncated",
			result:       strings.Repeat("x", call.MaxResultLen+100),
			wantState:    call.Complete,
			wantTruncate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := &call.Call{}
			c.Complete(tt.result, 10)

			if c.State != tt.wantState {
				t.Errorf("State = %q, want %q", c.State, tt.wantState)
			}

			if c.Error != nil {
				t.Errorf("Error = %v, want nil", c.Error)
			}

			// ResultTokens must be positive for non-empty results.
			if tt.result != "" && c.ResultTokens <= 0 {
				t.Errorf("ResultTokens = %d, want > 0 for non-empty result", c.ResultTokens)
			}

			if tt.wantTruncate {
				if !strings.HasSuffix(c.Preview, "...") {
					t.Errorf("Result = %q, want truncation marker for long result", c.Preview)
				}
			} else if tt.result != "" && c.Preview != tt.result {
				t.Errorf("Result = %q, want %q", c.Preview, tt.result)
			}
		})
	}
}

func TestFail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
	}{
		{
			name: "simple error",
			err:  errors.New("something went wrong"),
		},
		{
			name: "long error message",
			err:  errors.New(strings.Repeat("e", call.MaxResultLen+50)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := &call.Call{}
			c.Fail(tt.err, 10)

			if c.State != call.Error {
				t.Errorf("State = %q, want %q", c.State, call.Error)
			}

			if c.Error == nil {
				t.Error("Error = nil, want non-nil")
			}

			if !errors.Is(c.Error, tt.err) {
				t.Errorf("Error = %v, want %v", c.Error, tt.err)
			}

			if c.ResultTokens <= 0 {
				t.Errorf("ResultTokens = %d, want > 0", c.ResultTokens)
			}
		})
	}
}

func TestDisplayHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		callName         string
		argumentsDisplay string
		contains         []string
		absent           []string
	}{
		{
			name:             "with arguments",
			callName:         "read_file",
			argumentsDisplay: `{"path": "main.go"}`,
			contains:         []string{"[Tool: read_file]", `{"path": "main.go"}`},
		},
		{
			name:     "without arguments",
			callName: "list_tools",
			contains: []string{"[Tool: list_tools]"},
		},
		{
			name:             "empty arguments display",
			callName:         "noop",
			argumentsDisplay: "",
			contains:         []string{"[Tool: noop]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := call.Call{Name: tt.callName, ArgumentsDisplay: tt.argumentsDisplay}
			got := c.DisplayHeader()

			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("DisplayHeader() = %q, want it to contain %q", got, want)
				}
			}

			for _, unwanted := range tt.absent {
				if strings.Contains(got, unwanted) {
					t.Errorf("DisplayHeader() = %q, want it NOT to contain %q", got, unwanted)
				}
			}
		})
	}
}

func TestDisplayResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		c         call.Call
		wantEmpty bool
		contains  []string
	}{
		{
			name:      "pending state returns empty",
			c:         call.Call{State: call.Pending},
			wantEmpty: true,
		},
		{
			name:      "running state returns empty",
			c:         call.Call{State: call.Running},
			wantEmpty: true,
		},
		{
			name:     "complete state contains arrow",
			c:        call.Call{State: call.Complete, Preview: "file contents", ResultTokens: 3},
			contains: []string{"→"},
		},
		{
			name:     "complete state with empty result shows empty marker",
			c:        call.Call{State: call.Complete, Preview: "", ResultTokens: 0},
			contains: []string{"→ (empty)"},
		},
		{
			name:     "error state contains cross mark",
			c:        call.Call{State: call.Error, Error: errors.New("permission denied"), ResultTokens: 2},
			contains: []string{"✗"},
		},
		{
			name:     "complete state contains token info",
			c:        call.Call{State: call.Complete, Preview: "ok", ResultTokens: 5},
			contains: []string{"tokens"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.c.DisplayResult()

			if tt.wantEmpty {
				if got != "" {
					t.Errorf("DisplayResult() = %q, want empty string", got)
				}

				return
			}

			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("DisplayResult() = %q, want it to contain %q", got, want)
				}
			}
		})
	}
}

func TestForTranscript(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		c        call.Call
		contains []string
	}{
		{
			name:     "format includes tool name",
			c:        call.Call{Name: "bash", Arguments: map[string]any{"cmd": "ls"}},
			contains: []string{"→ bash:"},
		},
		{
			name:     "format includes arrow prefix",
			c:        call.Call{Name: "read_file"},
			contains: []string{"  → read_file:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.c.ForTranscript()

			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("ForTranscript() = %q, want it to contain %q", got, want)
				}
			}
		})
	}
}

func TestForLog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		c        call.Call
		contains []string
	}{
		{
			name:     "format includes tool name with arrow notation",
			c:        call.Call{Name: "bash", Arguments: map[string]any{"cmd": "ls"}},
			contains: []string{"[assistant->bash]"},
		},
		{
			name:     "format includes assistant prefix",
			c:        call.Call{Name: "read_file"},
			contains: []string{"[assistant->read_file]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.c.ForLog()

			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("ForLog() = %q, want it to contain %q", got, want)
				}
			}
		})
	}
}
