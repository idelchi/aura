package config

import "github.com/idelchi/aura/pkg/tokens"

// MCPFeature holds feature-level settings for MCP server filtering.
type MCPFeature struct {
	// Enabled is the default for --include-mcps. Glob patterns for servers to connect.
	// Empty = connect all enabled servers. CLI --include-mcps overrides this.
	Enabled []string `yaml:"enabled"`
	// Disabled is the default for --exclude-mcps. Glob patterns for servers to skip.
	// Empty = skip none. CLI --exclude-mcps overrides this.
	Disabled []string `yaml:"disabled"`
}

// ApplyDefaults is a no-op — MCPFeature has no default values.
func (m *MCPFeature) ApplyDefaults() error { return nil }

// PluginConfig holds feature-level settings for the plugin system.
type PluginConfig struct {
	// Dir overrides the plugin discovery directory for a config home.
	// Relative paths resolve against the home that declares them.
	// Empty = default "plugins/" subdirectory.
	// NOTE: This field exists so the YAML parser accepts it, but plugin
	// discovery uses ResolvePluginDir (peek) because it runs before feature merge.
	Dir string `yaml:"dir"`
	// Unsafe allows plugins to use os/exec and other restricted imports.
	// When false (default), plugins that import os/exec fail at load time.
	Unsafe bool `yaml:"unsafe"`
	// Include filters plugins by name. Empty = load all enabled plugins.
	Include []string `yaml:"include"`
	// Exclude filters plugins by name. Applied after Include.
	Exclude []string `yaml:"exclude"`
	// Config holds plugin config values. Global values are sent to all plugins;
	// Local values are keyed by plugin name and override Global.
	Config struct {
		Global map[string]any            `yaml:"global"`
		Local  map[string]map[string]any `yaml:"local"`
	} `yaml:"config"`
}

// ApplyDefaults is a no-op — PluginConfig has no default values.
func (p *PluginConfig) ApplyDefaults() error { return nil }

// Subagent holds configuration for the subagent (Task tool) system.
type Subagent struct {
	// MaxSteps caps the total number of LLM round-trips per subagent run.
	MaxSteps int `yaml:"max_steps"`
	// DefaultAgent is the agent name used when no agent is specified in the Task tool call.
	DefaultAgent string `yaml:"default_agent"`
}

// ApplyDefaults sets sane defaults for zero-valued fields.
func (s *Subagent) ApplyDefaults() error {
	if s.MaxSteps == 0 {
		s.MaxSteps = 25
	}

	return nil
}

// Estimation holds configuration for token estimation methods.
type Estimation struct {
	// Method selects the estimation algorithm: "rough", "tiktoken", "rough+tiktoken", "native".
	// "rough" = chars/divisor, "tiktoken" = tiktoken encoding, "rough+tiktoken" = max of both,
	// "native" = provider-specific tokenization (llamacpp /tokenize, ollama /api/generate).
	Method string `validate:"omitempty,oneof=rough tiktoken rough+tiktoken native" yaml:"method"`
	// Divisor is the chars-per-token divisor for rough estimation (default 4).
	Divisor int `yaml:"divisor"`
	// Encoding is the tiktoken encoding name (default "cl100k_base").
	Encoding string `yaml:"encoding"`
}

// NewEstimator creates a cached Estimator from this configuration.
func (e Estimation) NewEstimator() (*tokens.Estimator, error) {
	return tokens.NewEstimator(e.Method, e.Encoding, e.Divisor)
}

// ApplyDefaults sets sane defaults for zero-valued fields.
func (e *Estimation) ApplyDefaults() error {
	if e.Method == "" {
		e.Method = "rough+tiktoken"
	}

	if e.Divisor == 0 {
		e.Divisor = 4
	}

	if e.Encoding == "" {
		e.Encoding = "cl100k_base"
	}

	return nil
}
