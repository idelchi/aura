// Package cache provides domain-scoped file caching under a shared cache directory.
//
// Each domain maps to a subdirectory (e.g., "catwalk" → cache/catwalk/).
// No TTL — cached data is always valid until manually cleared via Clear()
// or bypassed with the NoCache option.
package cache

import (
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// Cache is the top-level cache manager rooted at a directory.
type Cache struct {
	dir     folder.Folder
	noCache bool
}

// New creates a cache rooted at writeHome/cache.
// If noCache is true, Read always returns a miss but Write still persists data.
func New(writeHome string, noCache bool) *Cache {
	return &Cache{
		dir:     folder.New(writeHome, "cache"),
		noCache: noCache,
	}
}

// Domain returns a sub-cache for a specific purpose (e.g., "catwalk", "models").
func (c *Cache) Domain(name string) *Domain {
	return &Domain{
		dir:     c.dir.Join(name),
		noCache: c.noCache,
	}
}

// Dir returns the root cache directory path.
func (c *Cache) Dir() string {
	return c.dir.Path()
}

// Clean removes the entire cache directory.
func (c *Cache) Clean() error {
	if !c.dir.Exists() {
		return nil
	}

	debug.Log("[cache] cleaning %s", c.dir.Path())

	return c.dir.Remove()
}
