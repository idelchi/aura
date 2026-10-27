package config_test

import (
	"testing"

	"github.com/idelchi/aura/internal/config"
)

// TestCompactionInputBudget derives a summarization request's input limit from the
// conversation's own compaction trigger and any output allowance.
func TestCompactionInputBudget(t *testing.T) {
	t.Parallel()

	output := func(n int) *int { return &n }

	for _, tc := range []struct {
		name      string
		cfg       config.Compaction
		window    int
		maxOutput *int
		want      int
		wantErr   bool
	}{
		{name: "absolute trigger wins", cfg: config.Compaction{Threshold: 80, MaxTokens: 104858}, window: 131072, want: 104858},
		{name: "absolute trigger without a known window", cfg: config.Compaction{Threshold: 80, MaxTokens: 4000}, want: 4000},
		{name: "percentage of the window", cfg: config.Compaction{Threshold: 80}, window: 32768, want: 26214},
		{name: "absolute trigger beyond a smaller window uses its threshold", cfg: config.Compaction{Threshold: 80, MaxTokens: 50000}, window: 8192, want: 6553},
		{name: "absolute trigger equal to the window uses its threshold", cfg: config.Compaction{Threshold: 80, MaxTokens: 8192}, window: 8192, want: 6553},
		{name: "output allowance after the threshold fallback", cfg: config.Compaction{Threshold: 80, MaxTokens: 50000}, window: 8192, maxOutput: output(4096), want: 4096},
		{name: "threshold beyond 100 is capped at the window", cfg: config.Compaction{Threshold: 1000}, window: 8192, want: 8192},
		{name: "non-positive threshold cannot derive a budget", cfg: config.Compaction{Threshold: 0}, window: 8192, wantErr: true},
		{name: "non-positive threshold as fallback for an oversized trigger", cfg: config.Compaction{Threshold: -5, MaxTokens: 50000}, window: 8192, wantErr: true},
		{name: "output allowance kept free", cfg: config.Compaction{Threshold: 80}, window: 32768, maxOutput: output(16384), want: 16384},
		{name: "small output allowance leaves the trigger", cfg: config.Compaction{Threshold: 80}, window: 32768, maxOutput: output(1024), want: 26214},
		{name: "unknown limit", cfg: config.Compaction{Threshold: 80}, want: 0},
		{name: "output allowance fills the window", cfg: config.Compaction{Threshold: 80}, window: 8192, maxOutput: output(8192), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, basis, err := tc.cfg.InputBudget(tc.window, tc.maxOutput)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}

			if !tc.wantErr && got != tc.want {
				t.Fatalf("InputBudget = %d (%s), want %d", got, basis, tc.want)
			}

			if !tc.wantErr && basis == "" {
				t.Fatal("budget has no explanation")
			}
		})
	}
}
