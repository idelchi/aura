package cache

import (
	"errors"
	"os"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// Domain is a scoped sub-cache within a named subdirectory.
type Domain struct {
	dir     folder.Folder
	noCache bool
}

// Read returns cached data for the given key.
// Returns nil and false on cache miss, read error, or when noCache is set.
func (d *Domain) Read(key string) ([]byte, bool) {
	if d.noCache {
		return nil, false
	}

	f := d.dir.WithFile(key)

	data, err := f.Read()
	if err != nil {
		return nil, false
	}

	debug.Log("[cache] hit: %s/%s", d.dir.Base(), key)

	return data, true
}

// Write persists data under the given key.
// Creates the domain directory if it doesn't exist.
func (d *Domain) Write(key string, data []byte) error {
	if err := d.dir.Create(); err != nil {
		return err
	}

	debug.Log("[cache] write: %s/%s (%d bytes)", d.dir.Base(), key, len(data))

	return d.dir.WithFile(key).Write(data, 0o644)
}

// Exists checks for a saved entry even when reads are bypassed.
func (d *Domain) Exists(key string) bool {
	return d.dir.WithFile(key).Exists()
}

// Delete invalidates one entry without clearing unrelated cached data.
func (d *Domain) Delete(key string) error {
	err := os.Remove(d.dir.WithFile(key).Path())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Clear removes all entries in this domain.
func (d *Domain) Clear() error {
	if !d.dir.Exists() {
		return nil
	}

	return d.dir.Remove()
}
