package config

// Snapshot configures automatic Git working-tree captures used by /undo.
type Snapshot struct {
	// Disabled skips Git snapshot initialization and creation. Nil inherits the
	// parent setting; explicit false re-enables snapshots in an overlay.
	Disabled *bool `yaml:"disabled"`
}

// IsDisabled reports whether automatic Git snapshots are explicitly disabled.
func (s Snapshot) IsDisabled() bool { return s.Disabled != nil && *s.Disabled }

// ApplyDefaults is a no-op: snapshots are enabled unless explicitly disabled.
func (s *Snapshot) ApplyDefaults() error { return nil }
