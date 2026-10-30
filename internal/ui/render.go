package ui

import (
	"fmt"
	"io"
	"strings"

	"github.com/idelchi/aura/pkg/llm/part"
	"github.com/idelchi/aura/pkg/llm/tool/call"
)

// Line reports where the output cursor is after rendering a part.
type Line int

const (
	// LineUnchanged means nothing was printed.
	LineUnchanged Line = iota
	// LineStart means the output ended with a newline.
	LineStart
	// LineOpen means the output ended mid-line.
	LineOpen
)

// lineAfter returns where printing text leaves the cursor.
func lineAfter(text string) Line {
	if strings.HasSuffix(text, "\n") {
		return LineStart
	}

	return LineOpen
}

// RenderPartAdded renders a newly added part to w (thinking/content/tool-header)
// and reports where the cursor ends.
func RenderPartAdded(w io.Writer, d Delta, p part.Part, verbose bool, prompt string) Line {
	line := LineUnchanged

	switch p.Type {
	case part.Thinking:
		if d.SectionBreak {
			fmt.Fprintln(w)

			line = LineStart
		}

		if verbose {
			if d.First {
				fmt.Fprintln(w)
				AssistantStyle.Fprint(w, prompt)
				ThinkingStyle.Fprint(w, "(thinking) ")

				line = LineOpen
			}

			if d.Text != "" {
				ThinkingStyle.Fprint(w, d.Text)

				line = lineAfter(d.Text)
			}
		}

	case part.Content:
		if d.First {
			fmt.Fprintln(w)
			AssistantStyle.Fprint(w, prompt)

			line = LineOpen
		} else if d.SectionBreak {
			fmt.Fprintln(w)

			line = LineStart
		}

		if d.Text != "" {
			ContentStyle.Fprint(w, d.Text)

			line = lineAfter(d.Text)
		}

	case part.Tool:
		// Tool calls are rendered in RenderPartUpdated when state transitions to Running.
	}

	return line
}

// RenderPartUpdated renders an updated part to w (thinking/content/tool-result)
// and reports where the cursor ends.
func RenderPartUpdated(w io.Writer, d Delta, p part.Part, verbose bool, prompt string) Line {
	line := LineUnchanged

	switch p.Type {
	case part.Thinking:
		if d.SectionBreak {
			fmt.Fprintln(w)

			line = LineStart
		}

		if verbose {
			if d.First {
				fmt.Fprintln(w)
				AssistantStyle.Fprint(w, prompt)
				ThinkingStyle.Fprint(w, "(thinking) ")

				line = LineOpen
			}

			if d.Text != "" {
				ThinkingStyle.Fprint(w, d.Text)

				line = lineAfter(d.Text)
			}
		}

	case part.Content:
		if d.First {
			fmt.Fprintln(w)
			AssistantStyle.Fprint(w, prompt)

			line = LineOpen
		} else if d.SectionBreak {
			fmt.Fprintln(w)

			line = LineStart
		}

		if d.Text != "" {
			ContentStyle.Fprint(w, d.Text)

			line = lineAfter(d.Text)
		}

	case part.Tool:
		if d.SectionBreak {
			fmt.Fprintln(w)

			line = LineStart
		}

		if p.Call != nil {
			switch p.Call.State {
			case call.Running:
				header, args := p.Call.DisplayFull()
				ToolStyle.Fprintf(w, "%s\n", header)

				if args != "" {
					ToolStyle.Fprintf(w, "%s\n", args)
				}

				line = LineStart
			case call.Complete:
				ToolResultStyle.Fprintln(w, p.Call.DisplayResult())

				line = LineStart
			case call.Error:
				ErrorStyle.Fprintln(w, p.Call.DisplayResult())

				line = LineStart
			}
		}
	}

	return line
}
