package model

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// ParameterCount represents a model's parameter count in absolute units.
// For example, an 8B model is stored as 8_000_000_000.
type ParameterCount int64

// String returns a human-readable parameter count (e.g., "8B", "70B", "1.5B", "567M").
// Zero value returns an empty string.
func (p ParameterCount) String() string {
	if p == 0 {
		return ""
	}

	abs := float64(p)

	// Sub-billion → show as millions.
	if abs < 1e9 {
		millions := abs / 1e6

		return formatCount(millions) + "M"
	}

	return formatCount(abs/1e9) + "B"
}

// Billions returns the parameter count in billions as a float.
func (p ParameterCount) Billions() float64 {
	return float64(p) / 1e9
}

// paramSizeRe matches strings like "8B", "70.6B", "1.5 B", "567M" (case-insensitive).
var paramSizeRe = regexp.MustCompile(`(?i)^(\d+\.?\d*)\s*([bm])$`)

// ParseParameterSize parses an explicit parameter size string (e.g., "8B", "70.6B", "567M").
// This is the format returned by the Ollama API's Details.ParameterSize field.
// Returns 0 if the string doesn't match.
func ParseParameterSize(s string) ParameterCount {
	s = strings.TrimSpace(s)

	m := paramSizeRe.FindStringSubmatch(s)
	if m == nil {
		return 0
	}

	f, err := strconv.ParseFloat(m[1], 64)
	if err != nil || f <= 0 {
		return 0
	}

	switch strings.ToLower(m[2]) {
	case "b":
		return ParameterCount(f * 1e9)
	case "m":
		return ParameterCount(f * 1e6)
	default:
		return 0
	}
}

// paramNameRe matches a parameter count segment in a model name.
// Looks for digits (with optional decimal) followed immediately by 'b'/'B',
// bounded by delimiters like ':', '-', or start/end of string.
// Examples: "qwen3:8b" → "8", "llama3.1:70b-instruct" → "70", "deepseek-r1:1.5b" → "1.5".
var paramNameRe = regexp.MustCompile(`(?i)(?:^|[:_-])(\d+\.?\d*)b(?:$|[^a-z])`)

// ParseParameterName extracts a parameter count from a model name string.
// Falls back to 0 if no pattern is found.
func ParseParameterName(name string) ParameterCount {
	m := paramNameRe.FindStringSubmatch(name)
	if m == nil {
		return 0
	}

	return parseBillions(m[1])
}

// formatCount formats a float as a clean string: whole → "8", fractional → "1.5" (1 decimal max).
func formatCount(v float64) string {
	if v == math.Trunc(v) {
		return strconv.FormatInt(int64(v), 10)
	}

	// Round to 1 decimal place for clean display.
	rounded := math.Round(v*10) / 10

	if rounded == math.Trunc(rounded) {
		return strconv.FormatInt(int64(rounded), 10)
	}

	return strconv.FormatFloat(rounded, 'f', 1, 64)
}

func parseBillions(s string) ParameterCount {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f <= 0 {
		return 0
	}

	return ParameterCount(f * 1e9)
}
