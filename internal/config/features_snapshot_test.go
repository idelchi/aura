package config_test

import (
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/files"
	"go.yaml.in/yaml/v4"
)

// TestSnapshotFeature loads the normal sample and preserves explicit false overlays.
func TestSnapshotFeature(t *testing.T) {
	var features config.Features
	if err := features.Load(files.Files{file.New("../../samples/config/features/snapshot.yaml")}); err != nil {
		t.Fatal(err)
	}
	if features.Snapshot.IsDisabled() || (config.Snapshot{}).IsDisabled() {
		t.Fatal("snapshots must default to enabled")
	}
	for _, value := range []bool{true, false} {
		if err := features.MergeFrom(config.Features{Snapshot: config.Snapshot{Disabled: new(value)}}); err != nil {
			t.Fatal(err)
		}
		if err := features.MergeFrom(config.Features{}); err != nil {
			t.Fatal(err)
		}
		if features.Snapshot.IsDisabled() != value {
			t.Fatalf("explicit disabled=%v was lost", value)
		}
	}
	if !strings.Contains(features.SummaryDisplay(), "disabled=false") {
		t.Fatal("feature summary omits snapshot state")
	}
	var node yaml.Node
	if err := yaml.Unmarshal([]byte("unknown: true"), &node); err != nil {
		t.Fatal(err)
	}
	if err := features.DecodeKey("snapshot", &node); err == nil {
		t.Fatal("snapshot feature accepted an unknown option")
	}
}
