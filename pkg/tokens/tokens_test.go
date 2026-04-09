package tokens_test

import (
	"testing"

	"github.com/idelchi/aura/pkg/tokens"
)

func TestRough(t *testing.T) {
	t.Parallel()

	estimator, err := tokens.NewEstimator("rough", "", 4)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		text string
		want int
	}{
		{
			name: "empty string returns zero",
			text: "",
			want: 0,
		},
		{
			name: "very short string returns at least one",
			text: "hi",
			want: 1,
		},
		{
			name: "three chars returns at least one",
			text: "hey",
			want: 1,
		},
		{
			name: "exactly four chars returns one",
			text: "four",
			want: 1,
		},
		{
			name: "eleven chars returns len divided by four",
			text: "hello world",
			want: 2,
		},
		{
			name: "forty chars returns ten",
			text: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			want: 10,
		},
		{
			name: "long string returns len divided by four",
			text: "The quick brown fox jumps over the lazy dog and keeps on running through the field",
			want: len("The quick brown fox jumps over the lazy dog and keeps on running through the field") / 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := estimator.EstimateLocal(tt.text)
			if got != tt.want {
				t.Errorf("Rough(%q) = %d, want %d", tt.text, got, tt.want)
			}
		})
	}
}
