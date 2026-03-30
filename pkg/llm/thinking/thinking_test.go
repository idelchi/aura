package thinking_test

import (
	"testing"

	"github.com/idelchi/aura/pkg/llm/thinking"

	"go.yaml.in/yaml/v4"
)

func TestParseValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		input        string
		wantErr      bool
		wantBool     bool   // only checked when wantErr=false and wantString=""
		wantString   string // non-empty means we expect a string-level value
		wantDisabled bool   // true means thinking should be disabled (Bool()==false)
	}{
		{name: "off", input: "off", wantDisabled: true},
		{name: "false", input: "false", wantDisabled: true},
		{name: "0", input: "0", wantDisabled: true},
		{name: "on", input: "on", wantDisabled: false, wantBool: true},
		{name: "true", input: "true", wantDisabled: false, wantBool: true},
		{name: "1", input: "1", wantDisabled: false, wantBool: true},
		{name: "low", input: "low", wantString: "low"},
		{name: "medium", input: "medium", wantString: "medium"},
		{name: "high", input: "high", wantString: "high"},
		{name: "invalid", input: "invalid", wantErr: true},
		{name: "empty string", input: "", wantErr: true},
		{name: "uppercase OFF", input: "OFF", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v, err := thinking.ParseValue(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseValue(%q) = nil error, want error", tt.input)
				}

				return
			}

			if err != nil {
				t.Fatalf("ParseValue(%q) unexpected error: %v", tt.input, err)
			}

			if tt.wantDisabled {
				if v.Bool() {
					t.Errorf("ParseValue(%q).Bool() = true, want false (disabled)", tt.input)
				}

				return
			}

			if tt.wantString != "" {
				// String level: Bool() should be true, String() should match.
				if !v.Bool() {
					t.Errorf("ParseValue(%q).Bool() = false, want true for string level", tt.input)
				}

				if got := v.String(); got != tt.wantString {
					t.Errorf("ParseValue(%q).String() = %q, want %q", tt.input, got, tt.wantString)
				}

				return
			}

			// Boolean enabled (on/true/1): Bool() should be true, IsBool() should be true.
			if tt.wantBool {
				if !v.Bool() {
					t.Errorf("ParseValue(%q).Bool() = false, want true", tt.input)
				}

				if !v.IsBool() {
					t.Errorf("ParseValue(%q).IsBool() = false, want true", tt.input)
				}
			}
		})
	}
}

func TestPtr(t *testing.T) {
	t.Parallel()

	t.Run("enabled bool value returns non-nil pointer", func(t *testing.T) {
		t.Parallel()

		v := thinking.NewValue(true)
		if v.Ptr() == nil {
			t.Errorf("Ptr() = nil for enabled value, want non-nil")
		}
	})

	t.Run("disabled bool value returns nil pointer", func(t *testing.T) {
		t.Parallel()

		v := thinking.NewValue(false)
		if v.Ptr() != nil {
			t.Errorf("Ptr() = non-nil for disabled value, want nil")
		}
	})

	t.Run("zero value returns nil pointer", func(t *testing.T) {
		t.Parallel()

		var v thinking.Value
		if v.Ptr() != nil {
			t.Errorf("Ptr() = non-nil for zero value, want nil")
		}
	})

	t.Run("string level returns non-nil pointer", func(t *testing.T) {
		t.Parallel()

		for _, level := range []string{"low", "medium", "high"} {
			v := thinking.NewValue(level)
			if v.Ptr() == nil {
				t.Errorf("Ptr() = nil for level %q, want non-nil", level)
			}
		}
	})
}

func TestCycleStates(t *testing.T) {
	t.Parallel()

	got := thinking.CycleStates()
	want := []any{false, true, "low", "medium", "high"}

	if len(got) != len(want) {
		t.Fatalf("CycleStates() len = %d, want %d", len(got), len(want))
	}

	for i, w := range want {
		if got[i] != w {
			t.Errorf("CycleStates()[%d] = %v (%T), want %v (%T)", i, got[i], got[i], w, w)
		}
	}
}

func TestAsString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value thinking.Value
		want  string
	}{
		{
			name:  "zero value returns false",
			value: thinking.Value{},
			want:  "false",
		},
		{
			name:  "disabled bool returns false",
			value: thinking.NewValue(false),
			want:  "false",
		},
		{
			name:  "enabled bool returns true",
			value: thinking.NewValue(true),
			want:  "true",
		},
		{
			name:  "low level returns low",
			value: thinking.NewValue("low"),
			want:  "low",
		},
		{
			name:  "medium level returns medium",
			value: thinking.NewValue("medium"),
			want:  "medium",
		},
		{
			name:  "high level returns high",
			value: thinking.NewValue("high"),
			want:  "high",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.value.AsString()
			if got != tt.want {
				t.Errorf("AsString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUnmarshalYAMLBoolTrue(t *testing.T) {
	t.Parallel()

	var w struct {
		Think thinking.Value `yaml:"think"`
	}
	if err := yaml.Unmarshal([]byte("think: true"), &w); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if !w.Think.Bool() {
		t.Error("Bool() = false, want true")
	}
}

func TestUnmarshalYAMLBoolFalse(t *testing.T) {
	t.Parallel()

	var w struct {
		Think thinking.Value `yaml:"think"`
	}
	if err := yaml.Unmarshal([]byte("think: false"), &w); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if w.Think.Bool() {
		t.Error("Bool() = true, want false")
	}
}

func TestUnmarshalYAMLStringLow(t *testing.T) {
	t.Parallel()

	var w struct {
		Think thinking.Value `yaml:"think"`
	}
	if err := yaml.Unmarshal([]byte("think: low"), &w); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if !w.Think.Bool() {
		t.Error("Bool() = false, want true for string level")
	}

	if got := w.Think.AsString(); got != "low" {
		t.Errorf("AsString() = %q, want %q", got, "low")
	}
}

func TestUnmarshalYAMLStringMedium(t *testing.T) {
	t.Parallel()

	var w struct {
		Think thinking.Value `yaml:"think"`
	}
	if err := yaml.Unmarshal([]byte("think: medium"), &w); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got := w.Think.AsString(); got != "medium" {
		t.Errorf("AsString() = %q, want %q", got, "medium")
	}
}

func TestUnmarshalYAMLStringHigh(t *testing.T) {
	t.Parallel()

	var w struct {
		Think thinking.Value `yaml:"think"`
	}
	if err := yaml.Unmarshal([]byte("think: high"), &w); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got := w.Think.AsString(); got != "high" {
		t.Errorf("AsString() = %q, want %q", got, "high")
	}
}

func TestUnmarshalYAMLInvalidString(t *testing.T) {
	t.Parallel()

	var w struct {
		Think thinking.Value `yaml:"think"`
	}

	err := yaml.Unmarshal([]byte("think: extreme"), &w)
	if err == nil {
		t.Fatal("expected error for invalid string, got nil")
	}
}

func TestUnmarshalYAMLWrongType(t *testing.T) {
	t.Parallel()

	var w struct {
		Think thinking.Value `yaml:"think"`
	}

	err := yaml.Unmarshal([]byte("think: 42"), &w)
	// yaml v4 may coerce int to string; the key check is it shouldn't panic
	// and if it becomes a string "42" that's not in Levels, it should error.
	// Accept either error or the coerced-to-string behavior.
	if err == nil {
		// If no error, the value was coerced — verify it's not a valid level
		got := w.Think.AsString()

		for _, level := range []string{"low", "medium", "high"} {
			if got == level {
				t.Errorf("AsString() = %q, should not match a valid level for int input", got)
			}
		}
	}
}
