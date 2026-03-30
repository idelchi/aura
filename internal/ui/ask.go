package ui

import (
	"encoding/json"
	"strconv"
	"strings"
)

// ResolveAskResponse parses the user's text input against ask options,
// converting numbered selections to labels and handling multi-select.
func ResolveAskResponse(text string, options []AskOption, multiSelect bool) string {
	if len(options) == 0 {
		return text
	}

	if multiSelect {
		var selected []string

		for p := range strings.SplitSeq(text, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}

			if n, err := strconv.Atoi(p); err == nil && n >= 1 && n <= len(options) {
				selected = append(selected, options[n-1].Label)
			} else {
				selected = append(selected, p)
			}
		}

		data, _ := json.Marshal(selected)

		return string(data)
	}

	// Single number → label
	if n, err := strconv.Atoi(strings.TrimSpace(text)); err == nil && n >= 1 && n <= len(options) {
		return options[n-1].Label
	}

	return text
}
