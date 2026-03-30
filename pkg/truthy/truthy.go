package truthy

import (
	"fmt"
	"slices"
	"strings"
)

var (
	trueValues  = []string{"true", "on", "yes", "enabled", "enable", "1"}
	falseValues = []string{"false", "off", "no", "disabled", "disable", "0"}
)

// Parse returns true/false for boolean-like strings, or an error.
func Parse(s string) (bool, error) {
	v := strings.ToLower(strings.TrimSpace(s))

	if slices.Contains(trueValues, v) {
		return true, nil
	}

	if slices.Contains(falseValues, v) {
		return false, nil
	}

	return false, fmt.Errorf("invalid boolean value %q: use on/off, true/false, yes/no, enable/disable", s)
}
