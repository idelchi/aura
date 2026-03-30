package plugins

import (
	"fmt"

	"github.com/Masterminds/semver/v3"
)

// IsSDKCompatible checks whether the host SDK version satisfies the plugin's
// requirement using tilde-range matching. A plugin vendoring SDK 0.1.0 is
// compatible with any host 0.1.x (same minor, patch >= plugin's patch).
//
// Returns nil when pluginVersion is empty (in-repo plugins without a vendored SDK).
func IsSDKCompatible(pluginVersion, hostVersion string) error {
	if pluginVersion == "" {
		return nil
	}

	c, err := semver.NewConstraint("~" + pluginVersion)
	if err != nil {
		return fmt.Errorf("invalid plugin SDK version %q: %w", pluginVersion, err)
	}

	v, err := semver.NewVersion(hostVersion)
	if err != nil {
		return fmt.Errorf("invalid host SDK version %q: %w", hostVersion, err)
	}

	if !c.Check(v) {
		return fmt.Errorf("plugin requires SDK ~%s, host has %s", pluginVersion, hostVersion)
	}

	return nil
}
