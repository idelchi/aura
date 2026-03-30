package config

// Vision holds configuration for the vision tool.
type Vision struct {
	// Agent is the name of the agent to use for vision calls.
	Agent string `yaml:"agent"`
	// Dimension is the max pixel dimension for image compression (default: 1024).
	Dimension int `yaml:"dimension"`
	// Quality is the JPEG compression quality 1-100 (default: 75).
	Quality int `yaml:"quality"`
}

// ApplyDefaults sets sane defaults for zero-valued fields.
func (v *Vision) ApplyDefaults() error {
	if v.Dimension == 0 {
		v.Dimension = 1024
	}

	if v.Quality == 0 {
		v.Quality = 75
	}

	return nil
}
