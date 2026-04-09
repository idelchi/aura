package ui_test

import (
	"testing"

	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/thinking"
)

func TestTokensDisplay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status ui.Status
		want   string
	}{
		{
			name:   "zero max returns empty",
			status: ui.Status{},
			want:   "",
		},
		{
			name: "normal values",
			status: ui.Status{
				Tokens: struct {
					Used    int
					Max     int
					Percent float64
				}{Used: 12400, Max: 131072, Percent: 10},
			},
			want: "tokens: 12.4k/131k (10%)",
		},
		{
			name: "large values",
			status: ui.Status{
				Tokens: struct {
					Used    int
					Max     int
					Percent float64
				}{Used: 500000, Max: 1000000, Percent: 50},
			},
			want: "tokens: 500k/1M (50%)",
		},
		{
			name: "zero used non-zero max",
			status: ui.Status{
				Tokens: struct {
					Used    int
					Max     int
					Percent float64
				}{Used: 0, Max: 131072, Percent: 0},
			},
			want: "tokens: 0/131k (0%)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.status.TokensDisplay()
			if got != tt.want {
				t.Errorf("TokensDisplay() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStatusLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status ui.Status
		hints  ui.DisplayHints
		want   string
	}{
		{
			name: "minimal agent and provider only",
			status: ui.Status{
				Agent:    "Base",
				Provider: "ollama",
			},
			want: "Base • think: false • ollama • 🔓",
		},
		{
			name: "full status",
			status: ui.Status{
				Agent:    "High",
				Mode:     "edit",
				Provider: "ollama",
				Model:    "qwen3:32b",
				Think:    thinking.NewValue("high"),
				Tokens: struct {
					Used    int
					Max     int
					Percent float64
				}{Used: 12400, Max: 131072, Percent: 10},
				Sandbox: struct {
					Enabled   bool
					Requested bool
				}{Enabled: true, Requested: true},
				Snapshots: true,
				Steps: struct {
					Current int
					Max     int
				}{Current: 3, Max: 300},
			},
			hints: ui.DisplayHints{Verbose: true, Auto: true},
			want:  "High • edit • qwen3:32b • think: high • ollama • step 3/300 • tokens: 12.4k/131k (10%) • 🔒 • 📸 • verbose • auto",
		},
		{
			name: "with iteration no sandbox",
			status: ui.Status{
				Agent:    "High",
				Provider: "ollama",
				Model:    "llama3",
				Think:    thinking.NewValue(false),
				Steps: struct {
					Current int
					Max     int
				}{Current: 5, Max: 100},
			},
			want: "High • llama3 • think: false • ollama • step 5/100 • 🔓",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.status.StatusLine(tt.hints)
			if got != tt.want {
				t.Errorf("StatusLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status ui.Status
		want   string
	}{
		{
			name:   "no tokens",
			status: ui.Status{},
			want:   "> ",
		},
		{
			name: "has tokens",
			status: ui.Status{
				Tokens: struct {
					Used    int
					Max     int
					Percent float64
				}{Used: 13000, Max: 131072, Percent: 10.5},
			},
			want: "[10.5%] > ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.status.Prompt()
			if got != tt.want {
				t.Errorf("Prompt() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWelcomeInfo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status ui.Status
		want   string
	}{
		{
			name: "with model",
			status: ui.Status{
				Agent:    "High",
				Provider: "ollama",
				Model:    "qwen3:32b",
			},
			want: "→ High (qwen3:32b)\n→ Provider: ollama\n→ Type /help for available commands",
		},
		{
			name: "without model",
			status: ui.Status{
				Agent:    "Base",
				Provider: "ollama",
			},
			want: "→ Base\n→ Provider: ollama\n→ Type /help for available commands",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.status.WelcomeInfo()
			if got != tt.want {
				t.Errorf("WelcomeInfo() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAssistantPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status ui.Status
		want   string
	}{
		{
			name:   "empty zero value",
			status: ui.Status{},
			want:   "Aura: ",
		},
		{
			name: "full",
			status: ui.Status{
				Model: "qwen3:32b",
				Think: thinking.NewValue("high"),
				Mode:  "edit",
			},
			want: "Aura (qwen3:32b, 🧠 high, edit): ",
		},
		{
			name: "think off explicitly",
			status: ui.Status{
				Model: "qwen3:32b",
				Think: thinking.NewValue(false),
			},
			want: "Aura (qwen3:32b): ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.status.AssistantPrompt()
			if got != tt.want {
				t.Errorf("AssistantPrompt() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUserPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status ui.Status
		want   string
	}{
		{
			name:   "empty",
			status: ui.Status{},
			want:   "You: ",
		},
		{
			name: "full",
			status: ui.Status{
				Model: "qwen3:32b",
				Mode:  "edit",
			},
			want: "You (qwen3:32b, edit): ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.status.UserPrompt()
			if got != tt.want {
				t.Errorf("UserPrompt() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestContextDisplay(t *testing.T) {
	t.Parallel()

	s := ui.Status{
		Tokens: struct {
			Used    int
			Max     int
			Percent float64
		}{Used: 12400, Max: 131072, Percent: 10},
	}

	got := s.ContextDisplay(42)
	want := "Context: 12.4k / 131k tokens (10%), 42 messages"

	if got != want {
		t.Errorf("ContextDisplay() = %q, want %q", got, want)
	}
}

func TestWindowDisplay(t *testing.T) {
	t.Parallel()

	s := ui.Status{
		Tokens: struct {
			Used    int
			Max     int
			Percent float64
		}{Max: 131072},
	}

	got := s.WindowDisplay()
	want := "Context window: 131k tokens"

	if got != want {
		t.Errorf("WindowDisplay() = %q, want %q", got, want)
	}
}
