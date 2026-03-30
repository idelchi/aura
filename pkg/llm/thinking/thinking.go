// Package thinking defines thinking levels, values, and management strategies for LLM reasoning.
package thinking

import (
	"fmt"
	"slices"
	"strconv"
)

// Level represents a thinking/reasoning level for LLM requests.
type Level string

const (
	// Low enables minimal thinking.
	Low Level = "low"
	// Medium enables moderate thinking.
	Medium Level = "medium"
	// High enables maximum thinking.
	High Level = "high"
)

// Levels contains all valid thinking level string values.
var Levels = []string{
	string(Low),
	string(Medium),
	string(High),
}

// Value represents a thinking configuration: bool (on/off) or string level.
// Zero value (nil Value) means thinking is disabled.
type Value struct {
	Value any // bool | string; nil = disabled
}

// NewValue creates a Value from a bool or string.
func NewValue(v any) Value {
	return Value{Value: v}
}

// IsBool returns true if the value is a boolean.
func (t Value) IsBool() bool {
	_, ok := t.Value.(bool)

	return ok
}

// IsString returns true if the value is a string.
func (t Value) IsString() bool {
	_, ok := t.Value.(string)

	return ok
}

// Bool returns true if thinking is enabled (true bool or any string level).
func (t Value) Bool() bool {
	switch v := t.Value.(type) {
	case bool:
		return v
	case string:
		return v != ""
	default:
		return false
	}
}

// String returns the value as a string.
// For string levels, returns the level directly. For bool true, returns "medium"
// (the default reasoning level). For bool false or nil, returns "".
func (t Value) String() string {
	switch v := t.Value.(type) {
	case string:
		return v
	case bool:
		if v {
			return string(Medium)
		}

		return "off"
	default:
		return "off"
	}
}

// Ptr returns a pointer to a copy of Value, or nil if thinking is disabled.
func (t Value) Ptr() *Value {
	if !t.Bool() {
		return nil
	}

	return &t
}

// UnmarshalYAML implements yaml.Unmarshaler for Value.
func (t *Value) UnmarshalYAML(unmarshal func(any) error) error {
	var b bool
	if err := unmarshal(&b); err == nil {
		t.Value = b

		return nil
	}

	var s string
	if err := unmarshal(&s); err == nil {
		if !slices.Contains(Levels, s) {
			return fmt.Errorf("invalid think value: %q (must be one of %v)", s, Levels)
		}

		t.Value = s

		return nil
	}

	return fmt.Errorf("think must be a boolean or string (one of %v)", Levels)
}

// ParseValue parses a CLI string into a Value.
// Accepts: "off", "false", "0" -> false; "on", "true", "1" -> true; "low", "medium", "high" -> string level.
func ParseValue(s string) (Value, error) {
	switch s {
	case "off", "false", "0":
		return NewValue(false), nil
	case "on", "true", "1":
		return NewValue(true), nil
	case string(Low), string(Medium), string(High):
		return NewValue(s), nil
	default:
		return Value{}, fmt.Errorf("invalid think value %q (must be off, on, low, medium, high)", s)
	}
}

// AsString returns a human-readable string representation.
func (t Value) AsString() string {
	if t.Value == nil {
		return "false"
	}

	if t.IsBool() {
		return strconv.FormatBool(t.Bool())
	}

	return t.String()
}

// CycleStates returns the ordered list of states for UI cycling.
// false -> true -> low -> medium -> high -> false.
func CycleStates() []any {
	return []any{false, true, string(Low), string(Medium), string(High)}
}

// Strategy defines how thinking blocks from prior turns are managed.
type Strategy string

const (
	// Keep preserves thinking blocks as-is (default, zero value "").
	Keep Strategy = ""
	// Strip removes all thinking blocks from prior turns.
	Strip Strategy = "strip"
	// Rewrite condenses thinking blocks via a dedicated agent.
	Rewrite Strategy = "rewrite"
)
