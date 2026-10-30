package config_test

import (
	"testing"

	"github.com/idelchi/aura/internal/config"
)

func TestProviderWireAPI(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		kind, api string
		valid     bool
	}{
		{"openai", "", true},
		{"openai", "responses", true},
		{"openai", "chat_completions", true},
		{"openai", "typo", false},
		{"anthropic", "responses", false},
	} {
		t.Run(tc.kind+"/"+tc.api, func(t *testing.T) {
			p := config.Provider{Type: tc.kind, API: tc.api}
			err := p.ApplyDefaults()
			if err == nil {
				c := config.Config{Providers: config.StringCollection[config.Provider]{"test": p}}
				err = c.Validate(map[config.Part]struct{}{config.PartProviders: {}})
			}
			if (err == nil) != tc.valid {
				t.Fatalf("valid = %t, error = %v", tc.valid, err)
			}
		})
	}
}
