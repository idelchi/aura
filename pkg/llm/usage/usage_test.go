package usage_test

import (
	"strings"
	"testing"

	"github.com/idelchi/aura/pkg/llm/usage"
)

func TestTotal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  int
		output int
		want   int
	}{
		{name: "zero value", input: 0, output: 0, want: 0},
		{name: "input only", input: 10, output: 0, want: 10},
		{name: "output only", input: 0, output: 5, want: 5},
		{name: "both", input: 100, output: 200, want: 300},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			u := usage.Usage{Input: tt.input, Output: tt.output}
			if got := u.Total(); got != tt.want {
				t.Errorf("Total() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestAdd(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		base    usage.Usage
		other   usage.Usage
		wantIn  int
		wantOut int
	}{
		{
			name:    "add to zero",
			base:    usage.Usage{Input: 0, Output: 0},
			other:   usage.Usage{Input: 10, Output: 20},
			wantIn:  10,
			wantOut: 20,
		},
		{
			name:    "accumulate",
			base:    usage.Usage{Input: 5, Output: 10},
			other:   usage.Usage{Input: 3, Output: 7},
			wantIn:  8,
			wantOut: 17,
		},
		{
			name:    "add zero to non-zero",
			base:    usage.Usage{Input: 50, Output: 100},
			other:   usage.Usage{Input: 0, Output: 0},
			wantIn:  50,
			wantOut: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			u := tt.base
			u.Add(tt.other)

			if u.Input != tt.wantIn {
				t.Errorf("Add() Input = %d, want %d", u.Input, tt.wantIn)
			}

			if u.Output != tt.wantOut {
				t.Errorf("Add() Output = %d, want %d", u.Output, tt.wantOut)
			}
		})
	}
}

func TestString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		u           usage.Usage
		mustContain []string
	}{
		{
			name:        "zero usage",
			u:           usage.Usage{Input: 0, Output: 0},
			mustContain: []string{"0"},
		},
		{
			name:        "non-zero usage contains input and output counts",
			u:           usage.Usage{Input: 123, Output: 456},
			mustContain: []string{"123", "456", "579"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.u.String()
			if got == "" {
				t.Errorf("String() returned empty string")
			}

			for _, sub := range tt.mustContain {
				if !strings.Contains(got, sub) {
					t.Errorf("String() = %q, want it to contain %q", got, sub)
				}
			}
		})
	}
}

func TestPercentOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		u     usage.Usage
		limit int
		want  float64
	}{
		{
			name:  "zero limit returns 100",
			u:     usage.Usage{Input: 50, Output: 50},
			limit: 0,
			want:  100.0,
		},
		{
			name:  "zero usage zero limit returns 100",
			u:     usage.Usage{Input: 0, Output: 0},
			limit: 0,
			want:  100.0,
		},
		{
			name:  "zero usage non-zero limit returns 0",
			u:     usage.Usage{Input: 0, Output: 0},
			limit: 1000,
			want:  0.0,
		},
		{
			name:  "half of limit",
			u:     usage.Usage{Input: 250, Output: 250},
			limit: 1000,
			want:  50.0,
		},
		{
			name:  "full limit",
			u:     usage.Usage{Input: 500, Output: 500},
			limit: 1000,
			want:  100.0,
		},
		{
			name:  "exceeds limit",
			u:     usage.Usage{Input: 600, Output: 600},
			limit: 1000,
			want:  120.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.u.PercentOf(tt.limit)
			if got != tt.want {
				t.Errorf("PercentOf(%d) = %f, want %f", tt.limit, got, tt.want)
			}
		})
	}
}
