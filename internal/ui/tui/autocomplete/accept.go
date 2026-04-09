package autocomplete

// Accept returns the modified text after accepting the current completion at cursorCol.
// Returns (newText, newCursorCol, ok). ok is false if there's no completion to accept.
func (c *Completer) Accept(text string, cursorCol int) (string, int, bool) {
	if cursorCol > len(text) {
		cursorCol = len(text)
	}

	hint := c.Complete(text, cursorCol)
	if hint == "" {
		return text, cursorCol, false
	}

	// Insert the completion at cursor position
	newText := text[:cursorCol] + hint + text[cursorCol:]
	newCol := cursorCol + len(hint)

	return newText, newCol, true
}
