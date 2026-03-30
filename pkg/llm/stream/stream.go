package stream

import (
	"fmt"

	"github.com/fatih/color"

	"github.com/idelchi/aura/pkg/spinner"
)

// Func is the callback for streaming content during chat completion.
type Func func(thinking, content string, done bool) error

// Streamer handles formatted streaming output with optional thinking display.
type Streamer struct {
	// Verbose controls whether thinking/reasoning output is displayed.
	Verbose bool
	// ThinkingStyle is the color style for thinking output.
	ThinkingStyle *color.Color
	// ContentStyle is the color style for content output.
	ContentStyle *color.Color
	// Preamble is text to display before streaming starts.
	Preamble string
	// PreambleStyle is the color style for the preamble.
	PreambleStyle *color.Color
	// Nil disables streaming entirely.
	Nil bool
}

// New creates a Streamer with the given options.
func New(options ...Option) *Streamer {
	streamer := &Streamer{
		Verbose: true,
	}

	for _, option := range options {
		option(streamer)
	}

	return streamer
}

type Spinner interface {
	Start()
	Stop()
}

// StreamResult holds the stream callback and cleanup function.
type StreamResult struct {
	Func Func
	Stop func()
}

func (s *Streamer) Stream(sp Spinner) StreamResult {
	if s.Nil {
		return StreamResult{Stop: func() {}}
	}

	if sp == nil {
		sp = &spinner.Noop{}
	}

	sp.Start()

	spinnerStopped := false
	stopSpinner := func() {
		if !spinnerStopped {
			sp.Stop()

			spinnerStopped = true
		}
	}

	preamblePrinted := false
	printPreamble := func() {
		if !preamblePrinted && s.Preamble != "" {
			if s.PreambleStyle != nil {
				fmt.Print(s.PreambleStyle.Sprint(s.Preamble))
			} else {
				fmt.Print(s.Preamble)
			}

			preamblePrinted = true
		}
	}

	thinkingStarted := false
	contentStarted := false

	return StreamResult{
		Stop: stopSpinner,
		Func: func(thinking, content string, done bool) error {
			willPrintThinking := thinking != "" && s.Verbose
			willPrintContent := content != ""

			if willPrintThinking || willPrintContent {
				stopSpinner()
				printPreamble()
			}

			if willPrintThinking {
				if !thinkingStarted {
					thinkingStarted = true
				}

				if s.ThinkingStyle != nil {
					fmt.Print(s.ThinkingStyle.Sprint(thinking))
				} else {
					fmt.Print(thinking)
				}
			}

			if willPrintContent {
				if !contentStarted {
					if thinkingStarted {
						fmt.Println()
					}

					contentStarted = true
				}

				if s.ContentStyle != nil {
					fmt.Print(s.ContentStyle.Sprint(content))
				} else {
					fmt.Print(content)
				}
			}

			if done {
				stopSpinner()
				printPreamble()
				fmt.Println()
			}

			return nil
		},
	}
}
