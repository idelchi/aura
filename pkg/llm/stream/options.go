package stream

import "github.com/fatih/color"

// Option configures a Streamer.
type Option func(*Streamer)

// WithNoStream disables streaming entirely.
func WithNoStream() Option {
	return func(s *Streamer) {
		s.Nil = true
	}
}

// WithVerbose controls whether thinking output is displayed.
func WithVerbose(show bool) Option {
	return func(s *Streamer) {
		s.Verbose = show
	}
}

// WithPreamble sets text to display before streaming begins.
func WithPreamble(preamble string) Option {
	return func(s *Streamer) {
		s.Preamble = preamble
	}
}

// WithThinkingStyle sets the color style for thinking output.
func WithThinkingStyle(style *color.Color) Option {
	return func(s *Streamer) {
		s.ThinkingStyle = style
	}
}

// WithPreambleStyle sets the color style for the preamble.
func WithPreambleStyle(style *color.Color) Option {
	return func(s *Streamer) {
		s.PreambleStyle = style
	}
}

// WithContentStyle sets the color style for content output.
func WithContentStyle(style *color.Color) Option {
	return func(s *Streamer) {
		s.ContentStyle = style
	}
}

// WithDefaultStyles applies the default color scheme.
func WithDefaultStyles() Option {
	return func(s *Streamer) {
		s.PreambleStyle = color.New(color.FgYellow)
		s.ContentStyle = color.New(color.Reset)
		s.ThinkingStyle = color.New(color.Faint)
	}
}
