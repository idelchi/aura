package spinner

import (
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
)

// Spinner wraps briandowns/spinner with consistent styling.
type Spinner struct {
	s *spinner.Spinner
}

// New creates a new Spinner with the given message.
func New(message string) *Spinner {
	const spinnerDelay = 100 * time.Millisecond

	s := spinner.New(spinner.CharSets[14], spinnerDelay, spinner.WithWriterFile(os.Stderr))

	s.Suffix = " " + message
	s.Color("cyan")

	return &Spinner{s: s}
}

// Start begins the spinner animation.
func (s *Spinner) Start() {
	s.s.Start()
}

// Stop stops the spinner without printing output.
func (s *Spinner) Stop() {
	s.s.Stop()
}

// Success stops the spinner and prints a success message.
func (s *Spinner) Success(message string) {
	s.s.Stop()

	if message != "" {
		color.Green("✓ " + message)
	}
}

// Fail stops the spinner and prints an error message.
func (s *Spinner) Fail(message string) {
	s.s.Stop()

	if message != "" {
		color.Red("✗ " + message)
	}
}

// Update changes the spinner message while running.
func (s *Spinner) Update(message string) {
	s.s.Lock()
	s.s.Suffix = " " + message
	s.s.Unlock()
}

// Noop implements a no-op spinner.
type Noop struct{}

// Start does nothing.
func (s *Noop) Start() {}

// Stop does nothing.
func (s *Noop) Stop() {}
