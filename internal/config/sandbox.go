package config

import (
	"slices"
)

// Restrictions defines filesystem access restrictions for sandboxed execution.
type Restrictions struct {
	ReadOnly  []string `yaml:"ro"`
	ReadWrite []string `yaml:"rw"`
}

// SandboxFeature holds sandbox configuration, loaded as part of Features.
type SandboxFeature struct {
	Enabled      *bool        `yaml:"enabled"`      // nil = inherit, &true = enable, &false = disable
	Restrictions Restrictions `yaml:"restrictions"` // base paths (override semantics via mergo)
	Extra        Restrictions `yaml:"extra"`        // additional paths (override semantics, concatenated at read time)
}

// IsEnabled returns true if the sandbox is explicitly enabled.
func (s SandboxFeature) IsEnabled() bool {
	return s.Enabled != nil && *s.Enabled
}

// ApplyDefaults is a no-op — SandboxFeature has no default values.
func (s *SandboxFeature) ApplyDefaults() error { return nil }

// EffectiveRestrictions returns the merged restrictions by concatenating base Restrictions
// with Extra. Both fields individually follow override semantics via mergo; the
// concatenation happens here at resolution time.
func (s SandboxFeature) EffectiveRestrictions() Restrictions {
	return Restrictions{
		ReadOnly:  append(slices.Clone(s.Restrictions.ReadOnly), s.Extra.ReadOnly...),
		ReadWrite: append(slices.Clone(s.Restrictions.ReadWrite), s.Extra.ReadWrite...),
	}
}
