// Package usage defines token usage tracking.
package usage

import "github.com/MakeNowJust/heredoc/v2"

// Usage represents token counts for a completion.
type Usage struct {
	// Input is the number of prompt tokens.
	Input int `json:"input"`
	// Output is the number of completion tokens.
	Output int `json:"output"`
}

// Total returns the sum of input and output tokens.
func (u Usage) Total() int {
	return u.Input + u.Output
}

// Add accumulates another Usage into this one.
func (u *Usage) Add(other Usage) {
	u.Input += other.Input
	u.Output += other.Output
}

// String returns a formatted token usage summary.
func (u Usage) String() string {
	return heredoc.Docf(`
		Input tokens: %d
		Output tokens: %d
		Total tokens: %d`,
		u.Input,
		u.Output,
		u.Total(),
	)
}

// PercentOf calculates token usage as a percentage of the given limit.
func (u Usage) PercentOf(limit int) float64 {
	if limit == 0 {
		return 100
	}

	return (float64(u.Total()) / float64(limit)) * 100
}
