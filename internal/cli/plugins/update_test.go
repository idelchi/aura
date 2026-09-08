package plugins

import (
	"bytes"
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/pkg/gitutil"
)

// TestUpdateAllFailure must not report a failed Git pack as absent or successful.
func TestUpdateAllFailure(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	pp := config.StringCollection[config.Plugin]{
		"broken": {Name: "broken", OriginDir: t.TempDir(), Origin: gitutil.Origin{URL: "https://example.invalid/repo"}},
	}
	if err := updateAll(&output, pp, false); err == nil {
		t.Fatal("update failure was swallowed")
	}
	if strings.Contains(output.String(), "No git-sourced") {
		t.Fatal("failed pack was reported as absent")
	}
}
