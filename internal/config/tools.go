package config

// Tools defines which tools are enabled or disabled for an agent or mode.
type Tools struct {
	// Enabled is the list of explicitly enabled tools.
	Enabled []string
	// Disabled is the list of explicitly disabled tools.
	Disabled []string
	// Policy controls which tool calls are auto-approved, require confirmation, or are blocked.
	Policy ToolPolicy `yaml:"policy"`
}
