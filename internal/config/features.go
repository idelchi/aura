package config

import (
	"fmt"

	"github.com/idelchi/aura/internal/config/merge"
	"github.com/idelchi/godyl/pkg/path/files"

	"go.yaml.in/yaml/v4"
)

// Features holds all feature-level configuration, loaded from .aura/config/features/*.yaml.
// Each YAML file uses a top-level key matching a feature name (compaction, title, thinking, vision, tools).
// YAML tags match the keys used in feature files and agent frontmatter overrides.
type Features struct {
	Compaction    Compaction     `yaml:"compaction"`
	Title         Title          `yaml:"title"`
	Thinking      Thinking       `yaml:"thinking"`
	Vision        Vision         `yaml:"vision"`
	Embeddings    Embeddings     `yaml:"embeddings"`
	ToolExecution ToolExecution  `yaml:"tools"`
	STT           STT            `yaml:"stt"`
	TTS           TTS            `yaml:"tts"`
	Sandbox       SandboxFeature `yaml:"sandbox"`
	Subagent      Subagent       `yaml:"subagent"`
	PluginConfig  PluginConfig   `yaml:"plugins"`
	MCP           MCPFeature     `yaml:"mcp"`
	Estimation    Estimation     `yaml:"estimation"`
	Guardrail     Guardrail      `yaml:"guardrail"`
}

// featureDef binds a YAML key to its decode target and post-load defaults function.
type featureDef struct {
	decode func(f *Features, node *yaml.Node) error
	apply  func(f *Features) error
}

// featureRegistry maps YAML top-level keys to their decode and apply-defaults functions.
// Adding a new feature requires one entry here (plus the struct field above).
var featureRegistry = map[string]featureDef{
	"compaction": {
		decode: func(f *Features, n *yaml.Node) error { return n.Load(&f.Compaction, yaml.WithKnownFields()) },
		apply:  func(f *Features) error { return f.Compaction.ApplyDefaults() },
	},
	"title": {
		decode: func(f *Features, n *yaml.Node) error { return n.Load(&f.Title, yaml.WithKnownFields()) },
		apply:  func(f *Features) error { return f.Title.ApplyDefaults() },
	},
	"thinking": {
		decode: func(f *Features, n *yaml.Node) error { return n.Load(&f.Thinking, yaml.WithKnownFields()) },
		apply:  func(f *Features) error { return f.Thinking.ApplyDefaults() },
	},
	"vision": {
		decode: func(f *Features, n *yaml.Node) error { return n.Load(&f.Vision, yaml.WithKnownFields()) },
		apply:  func(f *Features) error { return f.Vision.ApplyDefaults() },
	},
	"embeddings": {
		decode: func(f *Features, n *yaml.Node) error { return n.Load(&f.Embeddings, yaml.WithKnownFields()) },
		apply:  func(f *Features) error { return f.Embeddings.ApplyDefaults() },
	},
	"tools": {
		decode: func(f *Features, n *yaml.Node) error { return n.Load(&f.ToolExecution, yaml.WithKnownFields()) },
		apply:  func(f *Features) error { return f.ToolExecution.ApplyDefaults() },
	},
	"stt": {
		decode: func(f *Features, n *yaml.Node) error { return n.Load(&f.STT, yaml.WithKnownFields()) },
		apply:  func(f *Features) error { return f.STT.ApplyDefaults() },
	},
	"tts": {
		decode: func(f *Features, n *yaml.Node) error { return n.Load(&f.TTS, yaml.WithKnownFields()) },
		apply:  func(f *Features) error { return f.TTS.ApplyDefaults() },
	},
	"sandbox": {
		decode: func(f *Features, n *yaml.Node) error { return n.Load(&f.Sandbox, yaml.WithKnownFields()) },
		apply:  func(f *Features) error { return f.Sandbox.ApplyDefaults() },
	},
	"subagent": {
		decode: func(f *Features, n *yaml.Node) error { return n.Load(&f.Subagent, yaml.WithKnownFields()) },
		apply:  func(f *Features) error { return f.Subagent.ApplyDefaults() },
	},
	"plugins": {
		decode: func(f *Features, n *yaml.Node) error { return n.Load(&f.PluginConfig, yaml.WithKnownFields()) },
		apply:  func(f *Features) error { return f.PluginConfig.ApplyDefaults() },
	},
	"mcp": {
		decode: func(f *Features, n *yaml.Node) error { return n.Load(&f.MCP, yaml.WithKnownFields()) },
		apply:  func(f *Features) error { return f.MCP.ApplyDefaults() },
	},
	"estimation": {
		decode: func(f *Features, n *yaml.Node) error { return n.Load(&f.Estimation, yaml.WithKnownFields()) },
		apply:  func(f *Features) error { return f.Estimation.ApplyDefaults() },
	},
	"guardrail": {
		decode: func(f *Features, n *yaml.Node) error { return n.Load(&f.Guardrail, yaml.WithKnownFields()) },
		apply:  func(f *Features) error { return f.Guardrail.ApplyDefaults() },
	},
}

// MergeFrom applies non-zero fields from overlay on top of the receiver.
// Zero-valued overlay fields fall through to the receiver's existing value.
// Pointer fields (e.g. *int in BashTruncation) use nil = no override.
func (f *Features) MergeFrom(overlay Features) error {
	return merge.Merge(f, overlay)
}

// Load populates Features from YAML files. Each file may contain one or more
// top-level keys that dispatch to the corresponding struct field.
func (f *Features) Load(ff files.Files) error {
	for _, file := range ff {
		content, err := file.Read()
		if err != nil {
			return fmt.Errorf("reading feature config %s: %w", file.Path(), err)
		}

		var raw map[string]yaml.Node

		if err := yaml.Unmarshal(content, &raw); err != nil {
			return fmt.Errorf("parsing feature config %s: %w", file.Path(), err)
		}

		for key, node := range raw {
			if err := f.DecodeKey(key, &node); err != nil {
				return fmt.Errorf("decoding feature %q from %s: %w", key, file.Path(), err)
			}
		}
	}

	for _, def := range featureRegistry {
		if err := def.apply(f); err != nil {
			return err
		}
	}

	return nil
}

// DecodeKey dispatches a YAML node to the correct struct field based on the top-level key.
func (f *Features) DecodeKey(key string, node *yaml.Node) error {
	def, ok := featureRegistry[key]
	if !ok {
		return fmt.Errorf("unknown feature key %q", key)
	}

	return def.decode(f, node)
}
