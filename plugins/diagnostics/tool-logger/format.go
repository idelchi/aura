package tool_logger

// Truncate returns the first n characters of s, appending "..." if truncated.
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}

	return s[:n] + "..."
}
