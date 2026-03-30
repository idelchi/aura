package ui

import (
	"strings"

	"github.com/idelchi/aura/pkg/llm/part"
)

// PartTracker computes streaming deltas for content and thinking parts.
// It tracks the last-seen text length and part type, returning incremental
// text for display. All three non-TUI UIs (Simple, Headless, Web) embed
// this instead of maintaining their own tracking fields.
type PartTracker struct {
	lastContentLen  int
	lastThinkingLen int
	lastPartType    part.Type
}

// Delta holds the result of a single tracking computation.
type Delta struct {
	// Text is the incremental text to display (empty for tool parts).
	Text string
	// SectionBreak is true when transitioning between different part types.
	SectionBreak bool
	// PartType is the type of the incoming part.
	PartType part.Type
	// First is true when this is the first chunk for this part type
	// (the relevant counter was 0 before this call).
	First bool
}

// Added processes a newly added part. Call on MessagePartAdded events.
func (t *PartTracker) Added(p part.Part) Delta {
	return t.track(p)
}

// Updated processes an update to an existing part. Call on MessagePartUpdated events.
func (t *PartTracker) Updated(p part.Part) Delta {
	return t.track(p)
}

func (t *PartTracker) track(p part.Part) Delta {
	d := Delta{
		PartType:     p.Type,
		SectionBreak: t.lastPartType != "" && t.lastPartType != p.Type,
	}

	t.lastPartType = p.Type

	switch p.Type {
	case part.Content:
		d.First = t.lastContentLen == 0
		if len(p.Text) > t.lastContentLen {
			delta := p.Text[t.lastContentLen:]

			if d.First {
				delta = strings.TrimLeft(delta, " \t\n\r")
			}

			d.Text = delta
		}

		t.lastContentLen = len(p.Text)

	case part.Thinking:
		d.First = t.lastThinkingLen == 0
		if len(p.Text) > t.lastThinkingLen {
			delta := p.Text[t.lastThinkingLen:]

			if d.First {
				delta = strings.TrimLeft(delta, " \t\n\r")
			}

			d.Text = delta
		}

		t.lastThinkingLen = len(p.Text)

	case part.Tool:
		t.lastContentLen = 0
		t.lastThinkingLen = 0
	}

	return d
}

// Reset zeroes all counters. Call on MessageStarted and MessageFinalized.
func (t *PartTracker) Reset() {
	t.lastContentLen = 0
	t.lastThinkingLen = 0
	t.lastPartType = ""
}
