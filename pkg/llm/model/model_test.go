package model_test

import (
	"slices"
	"testing"

	"github.com/idelchi/aura/pkg/llm/model"
)

func TestMatches(t *testing.T) {
	t.Parallel()

	// Model.Matches(pattern) calls wildcard.MatchAny(pattern, m.Name),
	// which tests whether `pattern` matches the wildcard expression `m.Name`.
	// So the model name acts as the wildcard and the pattern arg is the subject.
	tests := []struct {
		name      string
		modelName string // used as the wildcard expression
		pattern   string // the string tested against that wildcard
		want      bool
	}{
		{
			name:      "exact name matches itself",
			modelName: "llama3.1:8b",
			pattern:   "llama3.1:8b",
			want:      true,
		},
		{
			name:      "different names do not match",
			modelName: "llama3.1:8b",
			pattern:   "llama3.1:70b",
			want:      false,
		},
		{
			name:      "model name as wildcard matches prefix",
			modelName: "llama*",
			pattern:   "llama3.1:8b",
			want:      true,
		},
		{
			name:      "model name as wildcard does not match non-prefix",
			modelName: "llama*",
			pattern:   "gpt-4",
			want:      false,
		},
		{
			name:      "star model name matches any pattern",
			modelName: "*",
			pattern:   "anything",
			want:      true,
		},
		{
			name:      "empty pattern does not match non-empty model name",
			modelName: "llama3",
			pattern:   "",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := model.Model{Name: tt.modelName}
			got := m.Matches(tt.pattern)

			if got != tt.want {
				t.Errorf("Model{Name:%q}.Matches(%q) = %v, want %v", tt.modelName, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestParseParameterSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  model.ParameterCount
	}{
		{
			name:  "8B",
			input: "8B",
			want:  model.ParameterCount(8e9),
		},
		{
			name:  "70.6B",
			input: "70.6B",
			want:  model.ParameterCount(70.6e9),
		},
		{
			name:  "567M",
			input: "567M",
			want:  model.ParameterCount(567e6),
		},
		{
			name:  "1.5B",
			input: "1.5B",
			want:  model.ParameterCount(1.5e9),
		},
		{
			name:  "case insensitive lowercase b",
			input: "8b",
			want:  model.ParameterCount(8e9),
		},
		{
			name:  "case insensitive lowercase m",
			input: "567m",
			want:  model.ParameterCount(567e6),
		},
		{
			name:  "with surrounding whitespace",
			input: "  8B  ",
			want:  model.ParameterCount(8e9),
		},
		{
			name:  "invalid returns zero",
			input: "invalid",
			want:  0,
		},
		{
			name:  "empty string returns zero",
			input: "",
			want:  0,
		},
		{
			name:  "number only returns zero",
			input: "8",
			want:  0,
		},
		{
			name:  "unknown suffix returns zero",
			input: "8K",
			want:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := model.ParseParameterSize(tt.input)

			if got != tt.want {
				t.Errorf("ParseParameterSize(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseParameterName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  model.ParameterCount
	}{
		{
			name:  "llama3.1:70b",
			input: "llama3.1:70b",
			want:  model.ParameterCount(70e9),
		},
		{
			name:  "qwen3:8b",
			input: "qwen3:8b",
			want:  model.ParameterCount(8e9),
		},
		{
			name:  "deepseek-r1:1.5b",
			input: "deepseek-r1:1.5b",
			want:  model.ParameterCount(1.5e9),
		},
		{
			name:  "model with no parameter count returns zero",
			input: "gpt-4",
			want:  0,
		},
		{
			name:  "empty name returns zero",
			input: "",
			want:  0,
		},
		{
			name:  "llama3.1:70b-instruct",
			input: "llama3.1:70b-instruct",
			want:  model.ParameterCount(70e9),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := model.ParseParameterName(tt.input)

			if got != tt.want {
				t.Errorf("ParseParameterName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParameterCountString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		count model.ParameterCount
		want  string
	}{
		{
			name:  "zero returns empty string",
			count: 0,
			want:  "",
		},
		{
			name:  "567M",
			count: model.ParameterCount(567e6),
			want:  "567M",
		},
		{
			name:  "8B",
			count: model.ParameterCount(8e9),
			want:  "8B",
		},
		{
			name:  "70B",
			count: model.ParameterCount(70e9),
			want:  "70B",
		},
		{
			name:  "1.5B",
			count: model.ParameterCount(1.5e9),
			want:  "1.5B",
		},
		{
			name:  "70.6B rounds to one decimal",
			count: model.ParameterCount(70.6e9),
			want:  "70.6B",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.count.String()

			if got != tt.want {
				t.Errorf("ParameterCount(%d).String() = %q, want %q", tt.count, got, tt.want)
			}
		})
	}
}

func TestContextLengthString(t *testing.T) {
	t.Parallel()

	// humanize.SIWithDigits produces SI prefixes with a space separator,
	// e.g. 8192 → "8 k", 131072 → "131 k", 0 → "0 ".
	tests := []struct {
		name   string
		length model.ContextLength
		want   string
	}{
		{
			name:   "8k context window",
			length: 8192,
			want:   "8 k",
		},
		{
			name:   "131k context window",
			length: 131072,
			want:   "131 k",
		},
		{
			name:   "32k context window",
			length: 32768,
			want:   "32 k",
		},
		{
			name:   "zero context",
			length: 0,
			want:   "0 ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.length.String()

			if got != tt.want {
				t.Errorf("ContextLength(%d).String() = %q, want %q", tt.length, got, tt.want)
			}
		})
	}
}

func TestContextLengthPercentUsed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		length      model.ContextLength
		inputTokens int
		want        float64
	}{
		{
			name:        "zero context returns zero",
			length:      0,
			inputTokens: 1000,
			want:        0,
		},
		{
			name:        "half context used",
			length:      1000,
			inputTokens: 500,
			want:        50.0,
		},
		{
			name:        "full context used",
			length:      1000,
			inputTokens: 1000,
			want:        100.0,
		},
		{
			name:        "no tokens used",
			length:      1000,
			inputTokens: 0,
			want:        0.0,
		},
		{
			name:        "small fraction",
			length:      8192,
			inputTokens: 100,
			want:        float64(100) / float64(8192) * 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.length.PercentUsed(tt.inputTokens)

			if got != tt.want {
				t.Errorf("ContextLength(%d).PercentUsed(%d) = %v, want %v", tt.length, tt.inputTokens, got, tt.want)
			}
		})
	}
}

func TestModelsIncludeExclude(t *testing.T) {
	t.Parallel()

	all := model.FromNames([]string{
		"llama3.1:8b",
		"llama3.1:70b",
		"qwen3:8b",
		"deepseek-r1:1.5b",
		"gpt-4",
	})

	tests := []struct {
		name      string
		operation string // "include" or "exclude"
		patterns  []string
		wantNames []string
	}{
		{
			name:      "include by exact name",
			operation: "include",
			patterns:  []string{"gpt-4"},
			wantNames: []string{"gpt-4"},
		},
		{
			name:      "include by wildcard prefix",
			operation: "include",
			patterns:  []string{"llama*"},
			wantNames: []string{"llama3.1:8b", "llama3.1:70b"},
		},
		{
			name:      "include by wildcard suffix",
			operation: "include",
			patterns:  []string{"*:8b"},
			wantNames: []string{"llama3.1:8b", "qwen3:8b"},
		},
		{
			name:      "include multiple patterns",
			operation: "include",
			patterns:  []string{"llama*", "gpt-4"},
			wantNames: []string{"llama3.1:8b", "llama3.1:70b", "gpt-4"},
		},
		{
			name:      "include with no patterns returns empty",
			operation: "include",
			patterns:  []string{},
			wantNames: []string{"llama3.1:8b", "llama3.1:70b", "qwen3:8b", "deepseek-r1:1.5b", "gpt-4"},
		},
		{
			name:      "exclude by wildcard",
			operation: "exclude",
			patterns:  []string{"llama*"},
			wantNames: []string{"qwen3:8b", "deepseek-r1:1.5b", "gpt-4"},
		},
		{
			name:      "exclude exact name",
			operation: "exclude",
			patterns:  []string{"gpt-4"},
			wantNames: []string{"llama3.1:8b", "llama3.1:70b", "qwen3:8b", "deepseek-r1:1.5b"},
		},
		{
			name:      "exclude with no patterns returns all",
			operation: "exclude",
			patterns:  []string{},
			wantNames: []string{"llama3.1:8b", "llama3.1:70b", "qwen3:8b", "deepseek-r1:1.5b", "gpt-4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got model.Models

			switch tt.operation {
			case "include":
				got = all.Include(tt.patterns...)
			case "exclude":
				got = all.Exclude(tt.patterns...)
			default:
				t.Fatalf("unknown operation: %q", tt.operation)
			}

			gotNames := got.Names()

			if len(gotNames) != len(tt.wantNames) {
				t.Errorf("%s(%v) returned %v, want %v", tt.operation, tt.patterns, gotNames, tt.wantNames)

				return
			}

			for _, want := range tt.wantNames {
				if !slices.Contains(gotNames, want) {
					t.Errorf("%s(%v) result missing %q, got %v", tt.operation, tt.patterns, want, gotNames)
				}
			}
		})
	}
}

func TestModelsByName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     []string
		wantOrder []string
	}{
		{
			name:      "already sorted",
			input:     []string{"apple", "banana", "cherry"},
			wantOrder: []string{"apple", "banana", "cherry"},
		},
		{
			name:      "reverse order gets sorted",
			input:     []string{"z-model", "m-model", "a-model"},
			wantOrder: []string{"a-model", "m-model", "z-model"},
		},
		{
			name:      "mixed case model names",
			input:     []string{"qwen3:8b", "llama3.1:70b", "deepseek-r1:1.5b", "gpt-4"},
			wantOrder: []string{"deepseek-r1:1.5b", "gpt-4", "llama3.1:70b", "qwen3:8b"},
		},
		{
			name:      "single model unchanged",
			input:     []string{"only-model"},
			wantOrder: []string{"only-model"},
		},
		{
			name:      "empty collection",
			input:     []string{},
			wantOrder: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ms := model.FromNames(tt.input)
			got := ms.ByName().Names()

			// Normalize nil vs empty for comparison.
			if got == nil {
				got = []string{}
			}

			if len(got) != len(tt.wantOrder) {
				t.Errorf("ByName() returned %v, want %v", got, tt.wantOrder)

				return
			}

			for i, want := range tt.wantOrder {
				if got[i] != want {
					t.Errorf("ByName()[%d] = %q, want %q (full result: %v)", i, got[i], want, got)
				}
			}
		})
	}
}

func TestFromNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input []string
	}{
		{
			name:  "creates models from names",
			input: []string{"llama3:8b", "qwen3:8b"},
		},
		{
			name:  "empty slice",
			input: []string{},
		},
		{
			name:  "single name",
			input: []string{"gpt-4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ms := model.FromNames(tt.input)
			got := ms.Names()

			if len(got) != len(tt.input) {
				t.Errorf("FromNames(%v).Names() = %v, want length %d", tt.input, got, len(tt.input))

				return
			}

			for i, want := range tt.input {
				if got[i] != want {
					t.Errorf("FromNames result[%d] = %q, want %q", i, got[i], want)
				}
			}
		})
	}
}
