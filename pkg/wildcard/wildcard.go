package wildcard

import (
	"regexp"
	"strings"
	"sync"
)

var cache sync.Map

// compiled returns a compiled regexp for the given pattern, caching the result.
func compiled(pattern string) *regexp.Regexp {
	if cached, ok := cache.Load(pattern); ok {
		return cached.(*regexp.Regexp)
	}

	re := regexp.MustCompile(pattern)
	cache.Store(pattern, re)

	return re
}

// Match reports whether the name matches the pattern.
//   - Only '*' is special; it matches any run of characters (excluding newlines).
//   - All other characters are literal. Matching is anchored to the full string.
//   - With zero inputs, it returns false.
func Match(name, pattern string) bool {
	quoted := regexp.QuoteMeta(pattern)
	glob := strings.ReplaceAll(quoted, `\*`, `.*`)

	re := compiled("^" + glob + "$")

	return re.MatchString(name)
}

// MatchMultiline reports whether the name matches the pattern.
// Like Match, but * also matches newlines (uses [\s\S]* instead of .*).
// Use this for matching content that may span multiple lines.
func MatchMultiline(name, pattern string) bool {
	quoted := regexp.QuoteMeta(pattern)
	glob := strings.ReplaceAll(quoted, `\*`, `[\s\S]*`)

	re := compiled("^" + glob + "$")

	return re.MatchString(name)
}

// MatchAny reports whether the name matches any of the patterns.
// See Match for details on matching behavior.
func MatchAny(name string, patterns ...string) bool {
	for _, pattern := range patterns {
		if Match(name, pattern) {
			return true
		}
	}

	return false
}

// MatchAnyExplicit reports whether the name matches any pattern,
// skipping the bare "*" wildcard. Use this for opt-in resolution where
// "*" means "all defaults" but should not satisfy explicit opt-in.
func MatchAnyExplicit(name string, patterns ...string) bool {
	for _, pattern := range patterns {
		if pattern == "*" {
			continue
		}

		if Match(name, pattern) {
			return true
		}
	}

	return false
}

// MatchAll reports whether the name matches all of the patterns.
// See Match for details on matching behavior.
func MatchAll(name string, patterns ...string) bool {
	for _, pattern := range patterns {
		if !Match(name, pattern) {
			return false
		}
	}

	return true
}
