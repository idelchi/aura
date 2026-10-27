package truncate

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// Truncate truncates the string s to a maximum length of n bytes,
// appending "..." if truncation occurs.
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}

	const suffix = "..."

	if n <= len(suffix) {
		return suffix[:n]
	}

	return strings.TrimSpace(Prefix(s, n-len(suffix))) + suffix
}

// Prefix returns the longest prefix of s that has at most n bytes and does not
// split a valid UTF-8 encoding. A character that ends exactly at n is kept.
// Invalid input is cut after at most utf8.UTFMax-1 bytes of backtracking, so a
// run of stray continuation bytes cannot empty the prefix. A non-positive n
// yields "".
func Prefix(s string, n int) string {
	if n >= len(s) {
		return s
	}

	if n <= 0 {
		return ""
	}

	// s[n] is the first excluded byte; while it continues a character, that
	// character straddles the cut and must be excluded too.
	for back := 0; back < utf8.UTFMax-1 && n > 0 && !utf8.RuneStart(s[n]); back++ {
		n--
	}

	return s[:n]
}

// FormatArgs formats a map as a truncated key=value string.
// Example: {file: "foo.go", line: 42} -> `file="foo.go" line=42`.
func FormatArgs(args map[string]any, maxLen int) string {
	if len(args) == 0 {
		return ""
	}

	var parts []string

	for k, v := range args {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}

	// Sort for consistent output
	slices.Sort(parts)

	result := strings.Join(parts, " ")

	return Truncate(result, maxLen)
}

// MapToJSON converts a map to indented JSON string.
func MapToJSON(m map[string]any) string {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", m)
	}

	return string(b)
}
