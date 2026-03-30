package config

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/yamlutil"
	"github.com/idelchi/godyl/pkg/path/files"
)

// ModelFilter controls which models appear in visual listings (aura models, /model).
type ModelFilter struct {
	Include []string `yaml:"include"`
	Exclude []string `yaml:"exclude"`
}

// Apply filters models using Include then Exclude with wildcard glob matching.
func (f ModelFilter) Apply(models model.Models) model.Models {
	return models.Include(f.Include...).Exclude(f.Exclude...)
}

// Capability represents a provider-level capability.
type Capability string

const (
	CapChat       Capability = "chat"
	CapEmbed      Capability = "embed"
	CapRerank     Capability = "rerank"
	CapTranscribe Capability = "transcribe"
	CapSynthesize Capability = "synthesize"
)

// Retry configures exponential-backoff retry for transient Chat() failures.
// Zero value (MaxAttempts: 0) means retry is disabled.
type Retry struct {
	// MaxAttempts is the number of retry attempts before giving up. 0 = disabled.
	MaxAttempts int `yaml:"max_attempts"`
	// BaseDelay is the initial delay between retries (Go duration). Default "1s".
	BaseDelay string `yaml:"base_delay"`
	// MaxDelay is the maximum delay between retries (Go duration). Default "30s".
	MaxDelay string `yaml:"max_delay"`
}

// Provider represents the configuration for an AI model provider.
type Provider struct {
	// URL is the API endpoint for the provider. Optional for providers with default endpoints (e.g., Anthropic).
	URL string
	// Type is the provider type (e.g., ollama, llamacpp, openrouter, openai, anthropic).
	Type string `validate:"required,oneof=ollama llamacpp openrouter openai anthropic google copilot codex"`
	// Token is the authentication token for API access.
	Token string
	// Models controls which models appear in visual listings.
	Models ModelFilter `yaml:"models"`
	// KeepAlive controls how long models stay loaded in VRAM after a request (Ollama only).
	// Uses Go duration syntax: "5m", "1h", "30s". Empty means provider default.
	KeepAlive string `yaml:"keep_alive"`
	// Timeout is the HTTP response header timeout for provider calls.
	// Uses Go duration syntax: "5m", "30s". Limits time waiting for response headers
	// without affecting long-running streaming responses.
	Timeout string `yaml:"timeout"`
	// Retry configures exponential-backoff retry for transient Chat() failures.
	// Disabled by default (MaxAttempts: 0). Only meaningful for cloud providers.
	Retry Retry `yaml:"retry"`
	// Capabilities declares what this provider supports (chat, embed, rerank, transcribe, synthesize).
	// Empty means all capabilities. Chat implies model listing.
	Capabilities []Capability `yaml:"capabilities"`
	// Source is the file path this provider was loaded from. Set by Load(), not from YAML.
	Source string `yaml:"-"`
	// AuthDirs is the ordered list of directories to search for stored auth tokens.
	// Populated by config.New(), not from YAML. Order: project auth dir, then global auth dir.
	AuthDirs []string `yaml:"-"`
}

// ApplyDefaults sets sane defaults for zero-valued fields.
func (p *Provider) ApplyDefaults() error {
	if p.Timeout == "" {
		p.Timeout = "5m"
	}

	return nil
}

// Can returns true if the provider supports the given capability.
// When Capabilities is empty, all capabilities are assumed.
func (p Provider) Can(cap Capability) bool {
	if len(p.Capabilities) == 0 {
		return true
	}

	return slices.Contains(p.Capabilities, cap)
}

// loadProviders populates a StringCollection of providers from the given files.
func loadProviders(ff files.Files) (StringCollection[Provider], error) {
	result := make(StringCollection[Provider])

	for _, f := range ff {
		content, err := f.Read()
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", f, err)
		}

		var temp map[string]Provider
		if err := yamlutil.StrictUnmarshal(content, &temp); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", f, err)
		}

		for key, val := range temp {
			if err := val.ApplyDefaults(); err != nil {
				return nil, fmt.Errorf("provider %q defaults: %w", key, err)
			}

			val.Token = os.ExpandEnv(val.Token)
			val.Source = f.Path()
			result[strings.ToLower(key)] = val
		}
	}

	// Load missing tokens from environment.
	for name, provider := range result {
		if provider.Token == "" {
			normalized := strings.ToUpper(name)

			normalized = strings.ReplaceAll(normalized, "-", "_")

			envVar := fmt.Sprintf("AURA_PROVIDERS_%s_TOKEN", normalized)
			if token := os.Getenv(envVar); token != "" {
				provider.Token = token
				result[name] = provider
			}
		}
	}

	return result, nil
}
