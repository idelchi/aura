package commands

// formatState returns a standardized "Name: enabled" / "Name: disabled" string.
func formatState(name string, enabled bool) string {
	if enabled {
		return name + ": enabled"
	}

	return name + ": disabled"
}
