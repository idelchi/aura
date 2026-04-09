package ui

import (
	"fmt"

	"github.com/idelchi/aura/pkg/llm/part"
	"github.com/idelchi/aura/pkg/llm/tool/call"
)

// RenderPartAdded renders a newly added part to stdout (thinking/content/tool-header).
func RenderPartAdded(d Delta, p part.Part, verbose bool, prompt string) {
	switch p.Type {
	case part.Thinking:
		if d.SectionBreak {
			fmt.Println()
		}

		if verbose {
			if d.First {
				fmt.Println()
				AssistantStyle.Print(prompt)
				ThinkingStyle.Print("(thinking) ")
			}

			if d.Text != "" {
				ThinkingStyle.Print(d.Text)
			}
		}

	case part.Content:
		if d.First {
			fmt.Println()
			AssistantStyle.Print(prompt)
		} else if d.SectionBreak {
			fmt.Println()
		}

		if d.Text != "" {
			ContentStyle.Print(d.Text)
		}

	case part.Tool:
		// Tool calls are rendered in RenderPartUpdated when state transitions to Running.
	}
}

// RenderPartUpdated renders an updated part to stdout (thinking/content/tool-result).
func RenderPartUpdated(d Delta, p part.Part, verbose bool, prompt string) {
	switch p.Type {
	case part.Thinking:
		if d.SectionBreak {
			fmt.Println()
		}

		if verbose {
			if d.First {
				fmt.Println()
				AssistantStyle.Print(prompt)
				ThinkingStyle.Print("(thinking) ")
			}

			if d.Text != "" {
				ThinkingStyle.Print(d.Text)
			}
		}

	case part.Content:
		if d.First {
			fmt.Println()
			AssistantStyle.Print(prompt)
		} else if d.SectionBreak {
			fmt.Println()
		}

		if d.Text != "" {
			ContentStyle.Print(d.Text)
		}

	case part.Tool:
		if d.SectionBreak {
			fmt.Println()
		}

		if p.Call != nil {
			switch p.Call.State {
			case call.Running:
				header, args := p.Call.DisplayFull()
				ToolStyle.Printf("%s\n", header)

				if args != "" {
					ToolStyle.Printf("%s\n", args)
				}
			case call.Complete:
				ToolResultStyle.Println(p.Call.DisplayResult())
			case call.Error:
				ErrorStyle.Println(p.Call.DisplayResult())
			}
		}
	}
}
