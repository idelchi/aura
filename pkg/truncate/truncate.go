package truncate

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
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

	return strings.TrimSpace(truncateUTF8(s, n-len(suffix))) + suffix
}

// truncateUTF8 returns a prefix of s having length no greater than n bytes,
// without splitting a multi-byte UTF-8 encoding.
// Reproduces tailscale.com/util/truncate.String exactly.
func truncateUTF8(s string, n int) string {
	if n >= len(s) {
		return s
	}

	// Back up past any continuation bytes (10xxxxxx).
	for n > 0 && s[n-1]&0xc0 == 0x80 {
		n--
	}

	// If we're at the start of a multi-byte sequence (11xxxxxx), skip it entirely.
	if n > 0 && s[n-1]&0xc0 == 0xc0 {
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
