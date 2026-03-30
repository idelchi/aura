package condition_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/idelchi/aura/internal/condition"
)

func TestCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		expr  string
		state condition.State
		want  bool
	}{
		// ── Literal boolean identifiers ──────────────────────────────────────
		// "true" and "false" are not keywords in the evaluator; they contain no
		// colon so they fall through to the unknown-condition path → false.
		{
			name:  "literal true is unknown condition returns false",
			expr:  "true",
			state: condition.State{},
			want:  false,
		},
		{
			name:  "literal false is unknown condition returns false",
			expr:  "false",
			state: condition.State{},
			want:  false,
		},

		// ── Boolean state flags ───────────────────────────────────────────────
		{
			name:  "auto true when state Auto is true",
			expr:  "auto",
			state: condition.State{Auto: true},
			want:  true,
		},
		{
			name:  "auto false when state Auto is false",
			expr:  "auto",
			state: condition.State{Auto: false},
			want:  false,
		},
		{
			name:  "todo_empty true when TodoEmpty is true",
			expr:  "todo_empty",
			state: condition.State{Todo: condition.TodoState{Empty: true}},
			want:  true,
		},
		{
			name:  "todo_empty false when TodoEmpty is false",
			expr:  "todo_empty",
			state: condition.State{Todo: condition.TodoState{Empty: false}},
			want:  false,
		},
		{
			name:  "todo_done true when TodoDone is true",
			expr:  "todo_done",
			state: condition.State{Todo: condition.TodoState{Done: true}},
			want:  true,
		},
		{
			name:  "todo_pending true when TodoPending is true",
			expr:  "todo_pending",
			state: condition.State{Todo: condition.TodoState{Pending: true}},
			want:  true,
		},

		// ── history (MessageCount) ────────────────────────────────────────────
		{
			name:  "history_gt true when count exceeds threshold",
			expr:  "history_gt:5",
			state: condition.State{MessageCount: 10},
			want:  true,
		},
		{
			name:  "history_gt false when count equals threshold",
			expr:  "history_gt:5",
			state: condition.State{MessageCount: 5},
			want:  false,
		},
		{
			name:  "history_gt false when count is below threshold",
			expr:  "history_gt:5",
			state: condition.State{MessageCount: 3},
			want:  false,
		},
		{
			name:  "history_lt true when count is below threshold",
			expr:  "history_lt:10",
			state: condition.State{MessageCount: 5},
			want:  true,
		},
		{
			name:  "history_lt false when count equals threshold",
			expr:  "history_lt:10",
			state: condition.State{MessageCount: 10},
			want:  false,
		},
		{
			name:  "history_lt false when count exceeds threshold",
			expr:  "history_lt:10",
			state: condition.State{MessageCount: 15},
			want:  false,
		},

		// ── context (TokensPercent) ───────────────────────────────────────────
		// context_above uses >= (not strictly >).
		{
			name:  "context_above true when percent exceeds threshold",
			expr:  "context_above:70",
			state: condition.State{Tokens: condition.TokensState{Percent: 80}},
			want:  true,
		},
		{
			name:  "context_above true when percent equals threshold",
			expr:  "context_above:70",
			state: condition.State{Tokens: condition.TokensState{Percent: 70}},
			want:  true,
		},
		{
			name:  "context_above false when percent is below threshold",
			expr:  "context_above:70",
			state: condition.State{Tokens: condition.TokensState{Percent: 50}},
			want:  false,
		},
		{
			name:  "context_below true when percent is below threshold",
			expr:  "context_below:30",
			state: condition.State{Tokens: condition.TokensState{Percent: 20}},
			want:  true,
		},
		{
			name:  "context_below false when percent equals threshold",
			expr:  "context_below:30",
			state: condition.State{Tokens: condition.TokensState{Percent: 30}},
			want:  false,
		},
		{
			name:  "context_below false when percent exceeds threshold",
			expr:  "context_below:30",
			state: condition.State{Tokens: condition.TokensState{Percent: 50}},
			want:  false,
		},

		// ── tool_errors ───────────────────────────────────────────────────────
		{
			name:  "tool_errors_gt true when errors exceed threshold",
			expr:  "tool_errors_gt:2",
			state: condition.State{Tools: condition.ToolsState{Errors: 5}},
			want:  true,
		},
		{
			name:  "tool_errors_gt false when errors equal threshold",
			expr:  "tool_errors_gt:2",
			state: condition.State{Tools: condition.ToolsState{Errors: 2}},
			want:  false,
		},
		{
			name:  "tool_errors_lt true when errors are below threshold",
			expr:  "tool_errors_lt:5",
			state: condition.State{Tools: condition.ToolsState{Errors: 2}},
			want:  true,
		},
		{
			name:  "tool_errors_lt false when errors equal threshold",
			expr:  "tool_errors_lt:5",
			state: condition.State{Tools: condition.ToolsState{Errors: 5}},
			want:  false,
		},

		// ── tool_calls ────────────────────────────────────────────────────────
		{
			name:  "tool_calls_gt true when calls exceed threshold",
			expr:  "tool_calls_gt:3",
			state: condition.State{Tools: condition.ToolsState{Calls: 10}},
			want:  true,
		},
		{
			name:  "tool_calls_gt false when calls are at threshold",
			expr:  "tool_calls_gt:3",
			state: condition.State{Tools: condition.ToolsState{Calls: 3}},
			want:  false,
		},
		{
			name:  "tool_calls_lt true when calls are below threshold",
			expr:  "tool_calls_lt:10",
			state: condition.State{Tools: condition.ToolsState{Calls: 4}},
			want:  true,
		},
		{
			name:  "tool_calls_lt false when calls equal threshold",
			expr:  "tool_calls_lt:10",
			state: condition.State{Tools: condition.ToolsState{Calls: 10}},
			want:  false,
		},

		// ── turns ─────────────────────────────────────────────────────────────
		{
			name:  "turns_gt true when turns exceed threshold",
			expr:  "turns_gt:1",
			state: condition.State{Turns: 5},
			want:  true,
		},
		{
			name:  "turns_gt false when turns equal threshold",
			expr:  "turns_gt:1",
			state: condition.State{Turns: 1},
			want:  false,
		},
		{
			name:  "turns_lt true when turns are below threshold",
			expr:  "turns_lt:10",
			state: condition.State{Turns: 3},
			want:  true,
		},
		{
			name:  "turns_lt false when turns equal threshold",
			expr:  "turns_lt:10",
			state: condition.State{Turns: 10},
			want:  false,
		},

		// ── compactions ───────────────────────────────────────────────────────
		{
			name:  "compactions_gt true when compactions exceed threshold",
			expr:  "compactions_gt:0",
			state: condition.State{Compactions: 1},
			want:  true,
		},
		{
			name:  "compactions_gt false when compactions equal threshold",
			expr:  "compactions_gt:0",
			state: condition.State{Compactions: 0},
			want:  false,
		},
		{
			name:  "compactions_lt true when compactions are below threshold",
			expr:  "compactions_lt:5",
			state: condition.State{Compactions: 2},
			want:  true,
		},
		{
			name:  "compactions_lt false when compactions equal threshold",
			expr:  "compactions_lt:5",
			state: condition.State{Compactions: 5},
			want:  false,
		},

		// ── iteration ─────────────────────────────────────────────────────────
		{
			name:  "iteration_gt true when iteration exceeds threshold",
			expr:  "iteration_gt:2",
			state: condition.State{Iteration: 3},
			want:  true,
		},
		{
			name:  "iteration_gt false when iteration equals threshold",
			expr:  "iteration_gt:2",
			state: condition.State{Iteration: 2},
			want:  false,
		},
		{
			name:  "iteration_lt true when iteration is below threshold",
			expr:  "iteration_lt:5",
			state: condition.State{Iteration: 1},
			want:  true,
		},
		{
			name:  "iteration_lt false when iteration equals threshold",
			expr:  "iteration_lt:5",
			state: condition.State{Iteration: 5},
			want:  false,
		},

		// ── model_context (ModelContextLength) ───────────────────────────────
		{
			name:  "model_context_gt true when context length exceeds threshold",
			expr:  "model_context_gt:8000",
			state: condition.State{Model: condition.ModelState{ContextLength: 128000}},
			want:  true,
		},
		{
			name:  "model_context_gt false when context length equals threshold",
			expr:  "model_context_gt:8000",
			state: condition.State{Model: condition.ModelState{ContextLength: 8000}},
			want:  false,
		},
		{
			name:  "model_context_lt true when context length is below threshold",
			expr:  "model_context_lt:128000",
			state: condition.State{Model: condition.ModelState{ContextLength: 8000}},
			want:  true,
		},
		{
			name:  "model_context_lt false when context length equals threshold",
			expr:  "model_context_lt:128000",
			state: condition.State{Model: condition.ModelState{ContextLength: 128000}},
			want:  false,
		},

		// ── model_has (Model.Capabilities) ───────────────────────────────────
		{
			name:  "model_has vision true when capability is present",
			expr:  "model_has:vision",
			state: condition.State{Model: condition.ModelState{Capabilities: map[string]bool{"vision": true}}},
			want:  true,
		},
		{
			name:  "model_has vision false when capability is absent",
			expr:  "model_has:vision",
			state: condition.State{Model: condition.ModelState{Capabilities: map[string]bool{}}},
			want:  false,
		},
		{
			name:  "model_has tools true when capability is present",
			expr:  "model_has:tools",
			state: condition.State{Model: condition.ModelState{Capabilities: map[string]bool{"tools": true}}},
			want:  true,
		},
		{
			name:  "model_has thinking true when capability is present",
			expr:  "model_has:thinking",
			state: condition.State{Model: condition.ModelState{Capabilities: map[string]bool{"thinking": true}}},
			want:  true,
		},
		{
			name:  "model_has nil map returns false",
			expr:  "model_has:vision",
			state: condition.State{Model: condition.ModelState{Capabilities: nil}},
			want:  false,
		},
		{
			name:  "model_has capability set to false returns false",
			expr:  "model_has:vision",
			state: condition.State{Model: condition.ModelState{Capabilities: map[string]bool{"vision": false}}},
			want:  false,
		},

		// ── model_params (Model.ParamCount) ──────────────────────────────────
		// Threshold is expressed in billions; 10B → 10_000_000_000.
		{
			name:  "model_params_gt true for 70B model above 10B threshold",
			expr:  "model_params_gt:10",
			state: condition.State{Model: condition.ModelState{ParamCount: 70_000_000_000}},
			want:  true,
		},
		{
			name:  "model_params_gt false when param count equals threshold exactly",
			expr:  "model_params_gt:10",
			state: condition.State{Model: condition.ModelState{ParamCount: 10_000_000_000}},
			want:  false,
		},
		{
			name:  "model_params_gt false for 7B model below 10B threshold",
			expr:  "model_params_gt:10",
			state: condition.State{Model: condition.ModelState{ParamCount: 7_000_000_000}},
			want:  false,
		},
		{
			name:  "model_params_lt true for 3B model below 7B threshold",
			expr:  "model_params_lt:7",
			state: condition.State{Model: condition.ModelState{ParamCount: 3_000_000_000}},
			want:  true,
		},
		{
			name:  "model_params_lt false for 70B model above 7B threshold",
			expr:  "model_params_lt:7",
			state: condition.State{Model: condition.ModelState{ParamCount: 70_000_000_000}},
			want:  false,
		},
		// Decimal threshold: 0.5B = 500_000_000.
		{
			name:  "model_params_gt decimal threshold 0.5B true for 1B model",
			expr:  "model_params_gt:0.5",
			state: condition.State{Model: condition.ModelState{ParamCount: 1_000_000_000}},
			want:  true,
		},
		{
			name:  "model_params_lt decimal threshold 0.5B true for 100M model",
			expr:  "model_params_lt:0.5",
			state: condition.State{Model: condition.ModelState{ParamCount: 100_000_000}},
			want:  true,
		},

		// ── Composition ("X and Y") ───────────────────────────────────────────
		{
			name:  "composition both true returns true",
			expr:  "auto and todo_empty",
			state: condition.State{Auto: true, Todo: condition.TodoState{Empty: true}},
			want:  true,
		},
		{
			name:  "composition first false returns false",
			expr:  "auto and todo_empty",
			state: condition.State{Auto: false, Todo: condition.TodoState{Empty: true}},
			want:  false,
		},
		{
			name:  "composition second false returns false",
			expr:  "auto and todo_empty",
			state: condition.State{Auto: true, Todo: condition.TodoState{Empty: false}},
			want:  false,
		},
		{
			name:  "composition both false returns false",
			expr:  "auto and todo_empty",
			state: condition.State{Auto: false, Todo: condition.TodoState{Empty: false}},
			want:  false,
		},
		{
			name:  "composition three terms all true returns true",
			expr:  "auto and todo_empty and todo_done",
			state: condition.State{Auto: true, Todo: condition.TodoState{Empty: true, Done: true}},
			want:  true,
		},
		{
			name:  "composition three terms one false returns false",
			expr:  "auto and todo_empty and todo_done",
			state: condition.State{Auto: true, Todo: condition.TodoState{Empty: true, Done: false}},
			want:  false,
		},
		{
			name:  "composition with parameterized term",
			expr:  "auto and history_gt:5",
			state: condition.State{Auto: true, MessageCount: 10},
			want:  true,
		},

		// ── Negation ("not X") ────────────────────────────────────────────────
		{
			name:  "not auto true when Auto is false",
			expr:  "not auto",
			state: condition.State{Auto: false},
			want:  true,
		},
		{
			name:  "not auto false when Auto is true",
			expr:  "not auto",
			state: condition.State{Auto: true},
			want:  false,
		},
		{
			name:  "not todo_empty true when TodoEmpty is false",
			expr:  "not todo_empty",
			state: condition.State{Todo: condition.TodoState{Empty: false}},
			want:  true,
		},
		{
			name:  "not parameterized condition true when condition is false",
			expr:  "not history_gt:5",
			state: condition.State{MessageCount: 3},
			want:  true,
		},
		{
			name:  "not with model_has true when capability absent",
			expr:  "not model_has:vision",
			state: condition.State{Model: condition.ModelState{Capabilities: map[string]bool{}}},
			want:  true,
		},
		// Negation in composition: each term is independently negated.
		{
			name:  "negation combined with composition",
			expr:  "not auto and todo_empty",
			state: condition.State{Auto: false, Todo: condition.TodoState{Empty: true}},
			want:  true,
		},

		// ── tokens_total (TokensTotal) ──────────────────────────────────────
		{
			name:  "tokens_total_gt true when total exceeds threshold",
			expr:  "tokens_total_gt:1000",
			state: condition.State{Tokens: condition.TokensState{Total: 5000}},
			want:  true,
		},
		{
			name:  "tokens_total_gt false when total equals threshold",
			expr:  "tokens_total_gt:1000",
			state: condition.State{Tokens: condition.TokensState{Total: 1000}},
			want:  false,
		},
		{
			name:  "tokens_total_lt true when total is below threshold",
			expr:  "tokens_total_lt:5000",
			state: condition.State{Tokens: condition.TokensState{Total: 1000}},
			want:  true,
		},
		{
			name:  "tokens_total_lt false when total equals threshold",
			expr:  "tokens_total_lt:5000",
			state: condition.State{Tokens: condition.TokensState{Total: 5000}},
			want:  false,
		},

		// ── model_is (Model.Name) ────────────────────────────────────────────
		{
			name:  "model_is true when name matches exactly",
			expr:  "model_is:qwen3:32b",
			state: condition.State{Model: condition.ModelState{Name: "qwen3:32b"}},
			want:  true,
		},
		{
			name:  "model_is true when name matches case-insensitively",
			expr:  "model_is:Qwen3:32B",
			state: condition.State{Model: condition.ModelState{Name: "qwen3:32b"}},
			want:  true,
		},
		{
			name:  "model_is false when name does not match",
			expr:  "model_is:llama3:8b",
			state: condition.State{Model: condition.ModelState{Name: "qwen3:32b"}},
			want:  false,
		},

		// ── Zero-value guards (unresolved model) ─────────────────────────────
		// When model metadata is 0 (unresolved), all model comparison conditions
		// return false — prevents false positives like "0 < 10e9 → true".
		{
			name:  "model_params_gt returns false when param count is 0",
			expr:  "model_params_gt:10",
			state: condition.State{Model: condition.ModelState{ParamCount: 0}},
			want:  false,
		},
		{
			name:  "model_params_lt returns false when param count is 0",
			expr:  "model_params_lt:10",
			state: condition.State{Model: condition.ModelState{ParamCount: 0}},
			want:  false,
		},
		{
			name:  "model_context_gt returns false when context length is 0",
			expr:  "model_context_gt:8000",
			state: condition.State{Model: condition.ModelState{ContextLength: 0}},
			want:  false,
		},
		{
			name:  "model_context_lt returns false when context length is 0",
			expr:  "model_context_lt:128000",
			state: condition.State{Model: condition.ModelState{ContextLength: 0}},
			want:  false,
		},

		// ── Unknown / malformed conditions ────────────────────────────────────
		{
			name:  "unknown condition returns false",
			expr:  "unknown_condition",
			state: condition.State{},
			want:  false,
		},
		{
			name:  "unknown parameterized condition returns false",
			expr:  "unknown_condition:42",
			state: condition.State{},
			want:  false,
		},
		{
			name:  "model_params_gt with non-numeric value returns false",
			expr:  "model_params_gt:notanumber",
			state: condition.State{Model: condition.ModelState{ParamCount: 1_000_000_000}},
			want:  false,
		},
		{
			name:  "history_gt with non-integer value returns false",
			expr:  "history_gt:abc",
			state: condition.State{MessageCount: 10},
			want:  false,
		},
		{
			name:  "empty expression returns false",
			expr:  "",
			state: condition.State{},
			want:  false,
		},
		// Bash condition — shell execution.
		{
			name:  "bash:true returns true",
			expr:  "bash:true",
			state: condition.State{},
			want:  true,
		},
		{
			name:  "bash:false returns false",
			expr:  "bash:false",
			state: condition.State{},
			want:  false,
		},
		{
			name:  "bash with multi-word command (first-colon split)",
			expr:  "bash:echo hello world",
			state: condition.State{},
			want:  true,
		},
		{
			name:  "not bash:false returns true",
			expr:  "not bash:false",
			state: condition.State{},
			want:  true,
		},
		{
			name:  "bash:true and auto composition",
			expr:  "bash:true and auto",
			state: condition.State{Auto: true},
			want:  true,
		},
		{
			name:  "bash:false and auto composition",
			expr:  "bash:false and auto",
			state: condition.State{Auto: true},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := condition.Check(tt.expr, tt.state)
			if got != tt.want {
				t.Errorf("Check(%q, state) = %v, want %v", tt.expr, got, tt.want)
			}
		})
	}
}

// TestCheck_Exists tests the "exists:" condition separately because it requires
// a real filesystem path that can only be determined at runtime via t.TempDir().
// These subtests are not parallelised relative to each other because they share
// no mutable state, but the parent is still parallel with the rest of the suite.
func TestCheck_Exists(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	existingFile := filepath.Join(dir, "target.txt")

	if err := os.WriteFile(existingFile, []byte("hello"), 0o600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	t.Run("exists returns true for existing file", func(t *testing.T) {
		t.Parallel()

		got := condition.Check("exists:"+existingFile, condition.State{})
		if !got {
			t.Errorf("Check(%q) = false, want true", "exists:"+existingFile)
		}
	})

	t.Run("exists returns false for nonexistent path", func(t *testing.T) {
		t.Parallel()

		nonexistent := filepath.Join(dir, "does_not_exist.txt")

		got := condition.Check("exists:"+nonexistent, condition.State{})
		if got {
			t.Errorf("Check(%q) = true, want false", "exists:"+nonexistent)
		}
	})

	t.Run("exists returns true for existing directory", func(t *testing.T) {
		t.Parallel()

		got := condition.Check("exists:"+dir, condition.State{})
		if !got {
			t.Errorf("Check(%q) = false, want true", "exists:"+dir)
		}
	})
}

func TestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		expr    string
		wantErr string // empty means no error expected
	}{
		// ── Valid conditions ──────────────────────────────────────────────────
		{name: "valid boolean", expr: "auto"},
		{name: "valid boolean todo_empty", expr: "todo_empty"},
		{name: "valid boolean todo_done", expr: "todo_done"},
		{name: "valid boolean todo_pending", expr: "todo_pending"},
		{name: "valid int history_gt", expr: "history_gt:5"},
		{name: "valid int context_above", expr: "context_above:70"},
		{name: "valid int context_below", expr: "context_below:30"},
		{name: "valid int model_context_gt", expr: "model_context_gt:8000"},
		{name: "valid int model_context_lt", expr: "model_context_lt:128000"},
		{name: "valid int tokens_total_gt", expr: "tokens_total_gt:1000"},
		{name: "valid int tokens_total_lt", expr: "tokens_total_lt:5000"},
		{name: "valid string model_has", expr: "model_has:vision"},
		{name: "valid string exists", expr: "exists:go.mod"},
		{name: "valid string model_is", expr: "model_is:qwen3:32b"},
		{name: "valid float model_params_gt", expr: "model_params_gt:10"},
		{name: "valid float model_params_lt", expr: "model_params_lt:0.5"},

		// ── Negation ─────────────────────────────────────────────────────────
		{name: "negation of boolean", expr: "not auto"},
		{name: "negation of parameterized", expr: "not history_gt:5"},

		// ── Composition ──────────────────────────────────────────────────────
		{name: "composition of two terms", expr: "auto and history_gt:5"},
		{name: "composition of three terms", expr: "auto and history_gt:5 and model_has:vision"},

		// ── Invalid conditions ───────────────────────────────────────────────
		{
			name:    "unknown condition",
			expr:    "unknown",
			wantErr: `unknown condition "unknown"`,
		},
		{
			name:    "parameterized condition missing value",
			expr:    "history_gt",
			wantErr: `condition "history_gt" requires a value`,
		},
		{
			name:    "non-integer value for int condition",
			expr:    "history_gt:abc",
			wantErr: `condition "history_gt": value "abc" is not an integer`,
		},
		{
			name:    "non-float value for float condition",
			expr:    "model_params_gt:abc",
			wantErr: `condition "model_params_gt": value "abc" is not a number`,
		},
		{
			name:    "empty string value for string condition",
			expr:    "model_has:",
			wantErr: `condition "model_has" requires a value`,
		},
		{
			name:    "unknown parameterized condition",
			expr:    "foobar:42",
			wantErr: `unknown condition "foobar"`,
		},
		{
			name:    "composition with one invalid term",
			expr:    "auto and unknown",
			wantErr: `unknown condition "unknown"`,
		},
		{
			name:    "negation of unknown condition",
			expr:    "not unknown",
			wantErr: `unknown condition "unknown"`,
		},
		// Bash condition validation.
		{
			name:    "bash with value is valid",
			expr:    "bash:echo hello",
			wantErr: "",
		},
		{
			name:    "bash without value is invalid",
			expr:    "bash:",
			wantErr: `condition "bash" requires a value`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := condition.Validate(tt.expr)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate(%q) = %v, want nil", tt.expr, err)
				}

				return
			}

			if err == nil {
				t.Errorf("Validate(%q) = nil, want error containing %q", tt.expr, tt.wantErr)

				return
			}

			if err.Error() != tt.wantErr {
				t.Errorf("Validate(%q) = %q, want %q", tt.expr, err.Error(), tt.wantErr)
			}
		})
	}
}
