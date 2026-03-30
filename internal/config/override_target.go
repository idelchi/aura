package config

// OverrideTarget is the unified struct that --override, --max-steps, and
// --token-budget all operate on. Features and Model in one YAML-addressable tree:
//
//	features.tools.max_steps=5
//	model.generation.temperature=0.7
//	model.context=200000
type OverrideTarget struct {
	Features Features `yaml:"features"`
	Model    Model    `yaml:"model"`
}
