package patch

import (
	"strings"
)

// unicodeNormalize maps common Unicode punctuation to ASCII equivalents.
// This handles copy-paste from rich-text editors with smart quotes, fancy dashes, etc.
var unicodeNormalize = map[rune]rune{
	// Dashes
	'\u2010': '-', // HYPHEN
	'\u2011': '-', // NON-BREAKING HYPHEN
	'\u2012': '-', // FIGURE DASH
	'\u2013': '-', // EN DASH
	'\u2014': '-', // EM DASH
	'\u2015': '-', // HORIZONTAL BAR
	'\u2212': '-', // MINUS SIGN

	// Single quotes
	'\u2018': '\'', // LEFT SINGLE QUOTATION MARK
	'\u2019': '\'', // RIGHT SINGLE QUOTATION MARK
	'\u201A': '\'', // SINGLE LOW-9 QUOTATION MARK
	'\u201B': '\'', // SINGLE HIGH-REVERSED-9 QUOTATION MARK

	// Double quotes
	'\u201C': '"', // LEFT DOUBLE QUOTATION MARK
	'\u201D': '"', // RIGHT DOUBLE QUOTATION MARK
	'\u201E': '"', // DOUBLE LOW-9 QUOTATION MARK
	'\u201F': '"', // DOUBLE HIGH-REVERSED-9 QUOTATION MARK

	// Spaces
	'\u00A0': ' ', // NO-BREAK SPACE
	'\u2002': ' ', // EN SPACE
	'\u2003': ' ', // EM SPACE
	'\u2004': ' ', // THREE-PER-EM SPACE
	'\u2005': ' ', // FOUR-PER-EM SPACE
	'\u2006': ' ', // SIX-PER-EM SPACE
	'\u2007': ' ', // FIGURE SPACE
	'\u2008': ' ', // PUNCTUATION SPACE
	'\u2009': ' ', // THIN SPACE
	'\u200A': ' ', // HAIR SPACE
	'\u202F': ' ', // NARROW NO-BREAK SPACE
	'\u205F': ' ', // MEDIUM MATHEMATICAL SPACE
	'\u3000': ' ', // IDEOGRAPHIC SPACE
}

// SeekLine finds a single line containing the pattern, starting from startIdx.
// Uses 4-level fuzzy matching.
func SeekLine(lines []string, pattern string, startIdx int) int {
	if pattern == "" {
		return startIdx
	}

	// Level 1: Exact contains
	for i := startIdx; i < len(lines); i++ {
		if strings.Contains(lines[i], pattern) {
			return i
		}
	}

	// Level 2: Trim right, then contains
	patternTrimR := strings.TrimRight(pattern, " \t")

	for i := startIdx; i < len(lines); i++ {
		lineTrimR := strings.TrimRight(lines[i], " \t")
		if strings.Contains(lineTrimR, patternTrimR) {
			return i
		}
	}

	// Level 3: Trim both, then contains
	patternTrim := strings.TrimSpace(pattern)

	for i := startIdx; i < len(lines); i++ {
		lineTrim := strings.TrimSpace(lines[i])
		if strings.Contains(lineTrim, patternTrim) {
			return i
		}
	}

	// Level 4: Unicode normalized, then contains
	patternNorm := normalizeUnicode(patternTrim)

	for i := startIdx; i < len(lines); i++ {
		lineNorm := normalizeUnicode(strings.TrimSpace(lines[i]))
		if strings.Contains(lineNorm, patternNorm) {
			return i
		}
	}

	return -1
}

// SeekSequence finds a sequence of lines in the file, starting from startIdx.
// Uses 4-level fuzzy matching.
func SeekSequence(lines, pattern []string, startIdx int) (int, bool) {
	if len(pattern) == 0 {
		return startIdx, true
	}

	if len(pattern) > len(lines) {
		return -1, false
	}

	// Level 1: Exact match
	if idx := findSequenceExact(lines, pattern, startIdx); idx >= 0 {
		return idx, true
	}

	// Level 2: Trim trailing whitespace
	if idx := findSequenceTrimRight(lines, pattern, startIdx); idx >= 0 {
		return idx, true
	}

	// Level 3: Trim both sides
	if idx := findSequenceTrimBoth(lines, pattern, startIdx); idx >= 0 {
		return idx, true
	}

	// Level 4: Unicode normalization
	if idx := findSequenceNormalized(lines, pattern, startIdx); idx >= 0 {
		return idx, true
	}

	return -1, false
}

func findSequenceExact(lines, pattern []string, startIdx int) int {
	for i := startIdx; i <= len(lines)-len(pattern); i++ {
		match := true

		for j := range pattern {
			if lines[i+j] != pattern[j] {
				match = false

				break
			}
		}

		if match {
			return i
		}
	}

	return -1
}

func findSequenceTrimRight(lines, pattern []string, startIdx int) int {
	for i := startIdx; i <= len(lines)-len(pattern); i++ {
		match := true

		for j := range pattern {
			lineTrim := strings.TrimRight(lines[i+j], " \t")

			patternTrim := strings.TrimRight(pattern[j], " \t")
			if lineTrim != patternTrim {
				match = false

				break
			}
		}

		if match {
			return i
		}
	}

	return -1
}

func findSequenceTrimBoth(lines, pattern []string, startIdx int) int {
	for i := startIdx; i <= len(lines)-len(pattern); i++ {
		match := true

		for j := range pattern {
			lineTrim := strings.TrimSpace(lines[i+j])

			patternTrim := strings.TrimSpace(pattern[j])
			if lineTrim != patternTrim {
				match = false

				break
			}
		}

		if match {
			return i
		}
	}

	return -1
}

func findSequenceNormalized(lines, pattern []string, startIdx int) int {
	// Pre-normalize pattern
	normalizedPattern := make([]string, len(pattern))
	for i, p := range pattern {
		normalizedPattern[i] = normalizeUnicode(strings.TrimSpace(p))
	}

	for i := startIdx; i <= len(lines)-len(pattern); i++ {
		match := true

		for j := range pattern {
			lineNorm := normalizeUnicode(strings.TrimSpace(lines[i+j]))
			if lineNorm != normalizedPattern[j] {
				match = false

				break
			}
		}

		if match {
			return i
		}
	}

	return -1
}

func normalizeUnicode(s string) string {
	var result strings.Builder
	result.Grow(len(s))

	for _, r := range s {
		if replacement, ok := unicodeNormalize[r]; ok {
			result.WriteRune(replacement)
		} else {
			result.WriteRune(r)
		}
	}

	return result.String()
}
