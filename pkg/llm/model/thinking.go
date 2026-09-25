package model

import (
	"fmt"
	"slices"
	"strings"

	"github.com/idelchi/aura/pkg/llm/thinking"
)

// NormalizeThinking validates known model restrictions, leaving unknown capabilities
// to the provider. On model switches, coerce replaces incompatible settings with
// off or auto; settings are preserved when capability metadata is unavailable.
func (m Model) NormalizeThinking(value thinking.Value, coerce bool) (thinking.Value, error) {
	if value.IsUnset() || value.IsOff() {
		return value, nil
	}

	if effort, ok := value.Effort(); ok && effort == thinking.None {
		return value, nil
	}

	if m.CapabilitiesKnown && !m.Capabilities.Thinking() {
		if coerce {
			return thinking.NewValue(false), nil
		}

		return thinking.Value{}, fmt.Errorf("model %q does not support thinking", m.Name)
	}

	if value.IsAuto() {
		return value, nil
	}

	effort, ok := value.Effort()
	if !ok {
		return value, nil
	}

	if len(m.ReasoningEfforts) > 0 {
		if slices.Contains(m.ReasoningEfforts, effort) {
			return value, nil
		}

		if coerce {
			return thinking.NewValue(true), nil
		}

		return thinking.Value{}, fmt.Errorf(
			"model %q does not support thinking effort %q (supported: %s)",
			m.Name,
			effort,
			formatReasoningEfforts(m.ReasoningEfforts),
		)
	}

	if m.CapabilitiesKnown && !m.Capabilities.ThinkingLevels() {
		if coerce {
			return thinking.NewValue(true), nil
		}

		return thinking.Value{}, fmt.Errorf("model %q supports thinking but not explicit thinking efforts", m.Name)
	}

	return value, nil
}

func formatReasoningEfforts(efforts []thinking.Effort) string {
	values := make([]string, 0, len(efforts))
	for _, effort := range efforts {
		values = append(values, string(effort))
	}

	return strings.Join(values, ", ")
}
