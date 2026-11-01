package tool

import "strings"

// HasUnparsedCall recognizes a response attempting the textual tool-call
// protocol. Quoted examples and prose mentioning its tags are ordinary text.
// It does not execute or try to repair the malformed payload.
func HasUnparsedCall(content string) bool {
	text := strings.TrimSpace(content)
	return strings.HasPrefix(text, "<tool_call>") || strings.HasPrefix(text, "<function=")
}
