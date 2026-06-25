// Package thinking defines thinking levels, values, and management strategies for LLM reasoning.
package thinking

import (
	"fmt"
	"strconv"
	"strings"
)

// Mode describes the caller's intent for model-side reasoning controls.
type Mode string

const (
	// ModeUnset means no thinking setting was configured.
	ModeUnset Mode = ""
	// ModeOff means thinking should be explicitly disabled where the provider supports it.
	ModeOff Mode = "off"
	// ModeAuto means thinking should use the provider or model default.
	ModeAuto Mode = "auto"
	// ModeEffort means thinking should use an explicit effort level.
	ModeEffort Mode = "effort"
)

// Effort represents a thinking/reasoning effort for LLM requests.
type Effort string

// Level is kept as a compatibility alias for older call sites.
type Level = Effort

const (
	// None requests no reasoning effort on providers that model "none" as an effort.
	None Effort = "none"
	// Minimal requests minimal reasoning effort.
	Minimal Effort = "minimal"
	// Low enables low thinking.
	Low Effort = "low"
	// Medium enables moderate thinking.
	Medium Effort = "medium"
	// High enables high thinking.
	High Effort = "high"
	// XHigh enables extra-high thinking where supported.
	XHigh Effort = "xhigh"
	// Max enables maximum thinking where supported.
	Max Effort = "max"
)

// Efforts contains all provider-neutral effort strings Aura accepts.
//
//nolint:gochecknoglobals
var Efforts = []string{
	string(None),
	string(Minimal),
	string(Low),
	string(Medium),
	string(High),
	string(XHigh),
	string(Max),
}

// Levels contains the legacy low/medium/high values.
//
//nolint:gochecknoglobals
var Levels = []string{
	string(Low),
	string(Medium),
	string(High),
}

// Value represents a thinking configuration: nil, bool, or string effort.
// nil means unset, bool false means off, bool true means auto, and string means
// explicit provider-neutral effort.
type Value struct {
	Value any // bool | string; nil = unset
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

// Mode returns the configured thinking mode.
func (t Value) Mode() Mode {
	switch v := t.Value.(type) {
	case nil:
		return ModeUnset
	case bool:
		if v {
			return ModeAuto
		}

		return ModeOff
	case string:
		switch v {
		case "":
			return ModeUnset
		case string(ModeOff):
			return ModeOff
		case string(ModeAuto):
			return ModeAuto
		default:
			if _, ok := ParseEffort(v); ok {
				return ModeEffort
			}

			return ModeUnset
		}
	default:
		return ModeUnset
	}
}

// IsUnset returns true when no thinking value was configured.
func (t Value) IsUnset() bool {
	return t.Mode() == ModeUnset
}

// IsOff returns true when thinking is explicitly disabled.
func (t Value) IsOff() bool {
	return t.Mode() == ModeOff
}

// IsAuto returns true when thinking should use provider/model defaults.
func (t Value) IsAuto() bool {
	return t.Mode() == ModeAuto
}

// Effort returns the explicit effort value, if one was configured.
func (t Value) Effort() (Effort, bool) {
	v, ok := t.Value.(string)
	if !ok {
		return "", false
	}

	return ParseEffort(v)
}

// Bool returns true if thinking is enabled (auto or any explicit effort).
func (t Value) Bool() bool {
	mode := t.Mode()

	return mode == ModeAuto || mode == ModeEffort
}

// String returns the value as a string.
// For string efforts, returns the effort directly. For bool true, returns "auto".
// For bool false or nil, returns "off".
func (t Value) String() string {
	if effort, ok := t.Effort(); ok {
		return string(effort)
	}

	switch t.Mode() {
	case ModeAuto:
		return string(ModeAuto)
	case ModeEffort:
		return "off"
	case ModeOff, ModeUnset:
		return string(ModeOff)
	default:
		return string(ModeOff)
	}
}

// Raw returns the bool or string value to pass through to providers such as Ollama.
func (t Value) Raw() any {
	if effort, ok := t.Effort(); ok {
		return string(effort)
	}

	switch t.Mode() {
	case ModeAuto:
		return true
	case ModeOff:
		return false
	case ModeUnset, ModeEffort:
		return nil
	default:
		return nil
	}
}

// Ptr returns a pointer to a copy of Value, or nil if thinking is unset.
func (t Value) Ptr() *Value {
	if t.IsUnset() {
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
		value, parseErr := ParseValue(s)
		if parseErr != nil {
			return parseErr
		}

		t.Value = value.Value

		return nil
	}

	return fmt.Errorf("think must be a boolean or string (one of %s)", strings.Join(ValidValues(), ", "))
}

// ParseValue parses a CLI string into a Value.
// Accepts: "off", "false", "0" -> false; "on", "auto", "true", "1" -> true;
// effort strings -> explicit effort.
func ParseValue(s string) (Value, error) {
	switch s {
	case "off", "false", "0":
		return NewValue(false), nil
	case "on", "auto", "true", "1":
		return NewValue(true), nil
	default:
		if _, ok := ParseEffort(s); ok {
			return NewValue(s), nil
		}

		return Value{}, fmt.Errorf("invalid think value %q (must be one of %s)", s, strings.Join(ValidValues(), ", "))
	}
}

// ParseEffort parses a provider-neutral effort string.
func ParseEffort(s string) (Effort, bool) {
	switch s {
	case string(None):
		return None, true
	case string(Minimal):
		return Minimal, true
	case string(Low):
		return Low, true
	case string(Medium):
		return Medium, true
	case string(High):
		return High, true
	case string(XHigh):
		return XHigh, true
	case string(Max):
		return Max, true
	default:
		return "", false
	}
}

// ValidValues returns user-facing values accepted by ParseValue.
func ValidValues() []string {
	return []string{
		string(ModeOff),
		"on",
		string(ModeAuto),
		string(None),
		string(Minimal),
		string(Low),
		string(Medium),
		string(High),
		string(XHigh),
		string(Max),
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
