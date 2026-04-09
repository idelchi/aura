// Package condition provides shared condition evaluation for /assert and custom injectors.
//
// Supports boolean conditions (todo_empty, auto), parameterized comparisons
// (history_gt:5, context_below:30), model conditions (model_has:vision,
// model_params_gt:10, model_context_lt:128000), filesystem conditions
// (exists:go.mod), shell conditions (bash:go build ./...), negation (not auto),
// and composition (history_gt:5 and history_lt:10).
package condition

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/godyl/pkg/path/file"
)

// TodoState holds todo-list boolean flags.
type TodoState struct {
	Empty   bool
	Done    bool
	Pending bool
}

// TokensState holds context-window token metrics.
type TokensState struct {
	Percent float64
	Total   int
}

// ToolsState holds cumulative tool invocation counts.
type ToolsState struct {
	Errors int
	Calls  int
}

// ModelState holds model metadata (zero values when model not yet resolved).
type ModelState struct {
	ParamCount    int64           // raw units (8e9 for 8B)
	ContextLength int             // max context tokens
	Capabilities  map[string]bool // "vision" → true, etc.
	Name          string          // e.g. "qwen3:32b", for model_is:<name> checks
}

// State is a structured snapshot of runtime values that conditions check against.
// Both /assert and custom injectors populate this from their respective contexts.
type State struct {
	Todo         TodoState
	Tokens       TokensState
	MessageCount int
	Tools        ToolsState
	Turns        int
	Compactions  int
	Iteration    int
	Auto         bool
	Model        ModelState
}

// Check evaluates a condition expression against the state.
//
// Supports:
//   - Boolean conditions: todo_empty, todo_done, todo_pending, auto
//   - Greater-than: history_gt:5, tool_errors_gt:3, context_above:70, model_context_gt:8000
//   - Less-than: history_lt:10, tool_errors_lt:5, context_below:30, model_context_lt:128000
//   - Model capabilities: model_has:vision, model_has:tools, model_has:thinking
//   - Model parameters: model_params_gt:10, model_params_lt:0.5 (in billions)
//   - Filesystem: exists:go.mod, exists:.aura/embeddings (files or directories)
//   - Negation: "not auto", "not model_has:vision"
//   - Composition: "model_has:tools and model_params_gt:10"
//
// All terms joined by "and" must be true. Unknown conditions evaluate to false.
func Check(expr string, s State) bool {
	terms := strings.SplitSeq(expr, " and ")
	for term := range terms {
		if !checkOne(strings.TrimSpace(term), s) {
			return false
		}
	}

	return true
}

// checkOne evaluates a single condition term with optional "not" prefix.
func checkOne(expr string, s State) bool {
	negate := false

	if after, ok := strings.CutPrefix(expr, "not "); ok {
		negate = true
		expr = after
	}

	result := eval(expr, s)

	if negate {
		return !result
	}

	return result
}

// Validate checks that a condition expression uses only known condition names
// and has parseable values. Both Validate and eval use the same registry, so
// adding a new condition is a single map entry.
//
// Validate checks syntax only: "exists:go.mod" validates that "exists" is a
// recognized name and the value is non-empty, but does NOT check the filesystem.
func Validate(expr string) error {
	terms := strings.SplitSeq(expr, " and ")
	for term := range terms {
		if err := validateOne(strings.TrimSpace(term)); err != nil {
			return err
		}
	}

	return nil
}

// validateOne validates a single condition term (with optional "not" prefix).
func validateOne(expr string) error {
	if after, ok := strings.CutPrefix(expr, "not "); ok {
		expr = after
	}

	return validateTerm(expr)
}

// conditionKind classifies how a condition's value is parsed and validated.
type conditionKind int

const (
	kindBoolean conditionKind = iota
	kindString
	kindInt
	kindFloat
)

// conditionDef pairs a condition's kind (for validation) with its eval function.
type conditionDef struct {
	kind conditionKind
	eval func(value string, s State) bool
}

// intGt returns an eval func that parses value as int and checks field > threshold.
func intGt(field func(State) int) func(string, State) bool {
	return func(value string, s State) bool {
		threshold, err := strconv.Atoi(value)
		if err != nil {
			return false
		}

		return field(s) > threshold
	}
}

// intLt returns an eval func that parses value as int and checks field < threshold.
func intLt(field func(State) int) func(string, State) bool {
	return func(value string, s State) bool {
		threshold, err := strconv.Atoi(value)
		if err != nil {
			return false
		}

		return field(s) < threshold
	}
}

// intGtGuarded is like intGt but returns false when the field is 0 (unresolved model data).
func intGtGuarded(name string, field func(State) int) func(string, State) bool {
	return func(value string, s State) bool {
		threshold, err := strconv.Atoi(value)
		if err != nil {
			return false
		}

		if field(s) == 0 {
			debug.Log("[condition] %s:%s: field is 0 (unresolved), returning false", name, value)

			return false
		}

		return field(s) > threshold
	}
}

// intLtGuarded is like intLt but returns false when the field is 0 (unresolved model data).
func intLtGuarded(name string, field func(State) int) func(string, State) bool {
	return func(value string, s State) bool {
		threshold, err := strconv.Atoi(value)
		if err != nil {
			return false
		}

		if field(s) == 0 {
			debug.Log("[condition] %s:%s: field is 0 (unresolved), returning false", name, value)

			return false
		}

		return field(s) < threshold
	}
}

// pctGte returns an eval func that parses value as int and checks field >= threshold.
// Used by context_above which uses >= (not >).
func pctGte(field func(State) float64) func(string, State) bool {
	return func(value string, s State) bool {
		threshold, err := strconv.Atoi(value)
		if err != nil {
			return false
		}

		return field(s) >= float64(threshold)
	}
}

// pctLt returns an eval func that parses value as int and checks field < threshold.
func pctLt(field func(State) float64) func(string, State) bool {
	return func(value string, s State) bool {
		threshold, err := strconv.Atoi(value)
		if err != nil {
			return false
		}

		return field(s) < float64(threshold)
	}
}

// floatGt parses value as float (billions), multiplies by 1e9, and checks field > threshold.
// Returns false when the field is 0 (unresolved model data).
func floatGt(name string, field func(State) int64) func(string, State) bool {
	return func(value string, s State) bool {
		fval, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return false
		}

		threshold := int64(math.Round(fval * 1e9))

		if field(s) == 0 {
			debug.Log("[condition] %s:%s: field is 0 (unresolved), returning false", name, value)

			return false
		}

		return field(s) > threshold
	}
}

// floatLt parses value as float (billions), multiplies by 1e9, and checks field < threshold.
// Returns false when the field is 0 (unresolved model data).
func floatLt(name string, field func(State) int64) func(string, State) bool {
	return func(value string, s State) bool {
		fval, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return false
		}

		threshold := int64(math.Round(fval * 1e9))

		if field(s) == 0 {
			debug.Log("[condition] %s:%s: field is 0 (unresolved), returning false", name, value)

			return false
		}

		return field(s) < threshold
	}
}

// bashCondition returns an eval function that runs the given command in a shell
// and returns true if it exits 0.
func bashCondition(timeout time.Duration) func(string, State) bool {
	return func(cmd string, _ State) bool {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		c := exec.CommandContext(ctx, "sh", "-c", cmd)

		err := c.Run()
		if err != nil {
			debug.Log("[condition] bash:%s failed: %v", cmd, err)

			return false
		}

		return true
	}
}

// registry maps condition names to their kind and eval function.
// Adding a new condition is a single entry here — both eval() and validateTerm() use this.
var registry = map[string]conditionDef{
	// Boolean (4).
	"todo_empty":   {kindBoolean, func(_ string, s State) bool { return s.Todo.Empty }},
	"todo_done":    {kindBoolean, func(_ string, s State) bool { return s.Todo.Done }},
	"todo_pending": {kindBoolean, func(_ string, s State) bool { return s.Todo.Pending }},
	"auto":         {kindBoolean, func(_ string, s State) bool { return s.Auto }},

	// String (4).
	"model_has": {kindString, func(v string, s State) bool { return s.Model.Capabilities[v] }},
	"exists":    {kindString, func(v string, _ State) bool { return file.New(v).Exists() }},
	"model_is":  {kindString, func(v string, s State) bool { return strings.EqualFold(s.Model.Name, v) }},
	"bash":      {kindString, bashCondition(120 * time.Second)},

	// Float — model params in billions (2).
	"model_params_gt": {kindFloat, floatGt("model_params_gt", func(s State) int64 { return s.Model.ParamCount })},
	"model_params_lt": {kindFloat, floatLt("model_params_lt", func(s State) int64 { return s.Model.ParamCount })},

	// Int — percentage context thresholds (2).
	// context_above uses >= (not >), context_below uses <.
	"context_above": {kindInt, pctGte(func(s State) float64 { return s.Tokens.Percent })},
	"context_below": {kindInt, pctLt(func(s State) float64 { return s.Tokens.Percent })},

	// Int — standard gt/lt (14).
	"history_gt":      {kindInt, intGt(func(s State) int { return s.MessageCount })},
	"history_lt":      {kindInt, intLt(func(s State) int { return s.MessageCount })},
	"tool_errors_gt":  {kindInt, intGt(func(s State) int { return s.Tools.Errors })},
	"tool_errors_lt":  {kindInt, intLt(func(s State) int { return s.Tools.Errors })},
	"tool_calls_gt":   {kindInt, intGt(func(s State) int { return s.Tools.Calls })},
	"tool_calls_lt":   {kindInt, intLt(func(s State) int { return s.Tools.Calls })},
	"turns_gt":        {kindInt, intGt(func(s State) int { return s.Turns })},
	"turns_lt":        {kindInt, intLt(func(s State) int { return s.Turns })},
	"compactions_gt":  {kindInt, intGt(func(s State) int { return s.Compactions })},
	"compactions_lt":  {kindInt, intLt(func(s State) int { return s.Compactions })},
	"iteration_gt":    {kindInt, intGt(func(s State) int { return s.Iteration })},
	"iteration_lt":    {kindInt, intLt(func(s State) int { return s.Iteration })},
	"tokens_total_gt": {kindInt, intGt(func(s State) int { return s.Tokens.Total })},
	"tokens_total_lt": {kindInt, intLt(func(s State) int { return s.Tokens.Total })},

	// Int — guarded gt/lt with zero-value check + debug.Log (2).
	"model_context_gt": {kindInt, intGtGuarded("model_context_gt", func(s State) int { return s.Model.ContextLength })},
	"model_context_lt": {kindInt, intLtGuarded("model_context_lt", func(s State) int { return s.Model.ContextLength })},
}

// validateTerm validates a single atomic condition (no negation, no composition).
func validateTerm(cond string) error {
	if def, ok := registry[cond]; ok {
		if def.kind == kindBoolean {
			return nil
		}

		return fmt.Errorf("condition %q requires a value", cond)
	}

	name, value, ok := strings.Cut(cond, ":")
	if !ok {
		return fmt.Errorf("unknown condition %q", cond)
	}

	def, ok := registry[name]
	if !ok {
		return fmt.Errorf("unknown condition %q", name)
	}

	switch def.kind {
	case kindString:
		if value == "" {
			return fmt.Errorf("condition %q requires a value", name)
		}
	case kindInt:
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("condition %q: value %q is not an integer", name, value)
		}
	case kindFloat:
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return fmt.Errorf("condition %q: value %q is not a number", name, value)
		}
	}

	return nil
}

// eval evaluates a single atomic condition (no negation, no composition).
func eval(cond string, s State) bool {
	if def, ok := registry[cond]; ok && def.kind == kindBoolean {
		return def.eval("", s)
	}

	name, value, ok := strings.Cut(cond, ":")
	if !ok {
		return false
	}

	def, ok := registry[name]
	if !ok {
		return false
	}

	return def.eval(value, s)
}
