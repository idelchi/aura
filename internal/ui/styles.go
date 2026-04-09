package ui

import "github.com/fatih/color"

// Shared terminal styles used by Simple and Headless backends.
var (
	UserStyle       = color.New(color.FgBlue, color.Bold)
	AssistantStyle  = color.New(color.FgYellow, color.Bold)
	ContentStyle    = color.New(color.FgWhite)
	ThinkingStyle   = color.New(color.Faint)
	ToolStyle       = color.New(color.FgCyan, color.Faint)
	ToolResultStyle = color.New(color.Faint)
	ErrorStyle      = color.New(color.FgRed, color.Bold)
	SyntheticStyle  = color.New(color.FgBlue)
)
