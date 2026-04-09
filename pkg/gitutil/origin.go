package gitutil

import (
	"fmt"

	"github.com/idelchi/aura/pkg/yamlutil"
	"github.com/idelchi/godyl/pkg/path/folder"

	"go.yaml.in/yaml/v4"
)

const originFile = ".origin.yaml"

// Origin tracks where an installable entity was sourced from.
type Origin struct {
	URL      string   `yaml:"url"`
	Ref      string   `yaml:"ref,omitempty"`
	Commit   string   `yaml:"commit,omitempty"`
	Subpaths []string `yaml:"subpaths,omitempty"`
}

// ReadOrigin reads the origin sidecar file from a directory.
// Returns a zero-value Origin and an error if the file does not exist.
func ReadOrigin(dir string) (Origin, error) {
	data, err := folder.New(dir).WithFile(originFile).Read()
	if err != nil {
		return Origin{}, fmt.Errorf("reading origin: %w", err)
	}

	var o Origin
	if err := yamlutil.StrictUnmarshal(data, &o); err != nil {
		return Origin{}, fmt.Errorf("parsing origin: %w", err)
	}

	return o, nil
}

// WriteOrigin writes the origin sidecar file to a directory.
func WriteOrigin(dir string, origin Origin) error {
	data, err := yaml.Marshal(origin)
	if err != nil {
		return fmt.Errorf("marshaling origin: %w", err)
	}

	return folder.New(dir).WithFile(originFile).Write(data, 0o644)
}

// HasOrigin returns true if the directory has a valid origin sidecar with a URL.
func HasOrigin(dir string) bool {
	o, err := ReadOrigin(dir)

	return err == nil && o.URL != ""
}

// FindOrigin walks up from dir looking for .origin.yaml.
// Returns the origin, the directory where it was found, and whether it was found.
func FindOrigin(dir string) (Origin, string, bool) {
	for d := folder.New(dir); ; d = d.Dir() {
		if origin, err := ReadOrigin(d.Path()); err == nil {
			return origin, d.Path(), true
		}

		if d.Dir() == d {
			break
		}
	}

	return Origin{}, "", false
}

// ShortCommit returns the first 12 characters of a commit hash.
func ShortCommit(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}

	return hash
}
