package config

import (
	"maps"
	"slices"
	"strings"

	"github.com/idelchi/godyl/pkg/path/file"
)

// Namer is satisfied by config entities that have a display name.
type Namer interface {
	Name() string
}

// Collection is a file-keyed collection of config entities.
// Used for Agents, Modes, Skills, CustomCommands, Systems.
type Collection[T Namer] map[file.File]T

// Get retrieves an entity by name (case-insensitive), returning nil if not found.
func (c Collection[T]) Get(name string) *T {
	for k, item := range c {
		if strings.EqualFold(item.Name(), name) {
			v := c[k]

			return &v
		}
	}

	return nil
}

// GetWithKey retrieves an entity by name (case-insensitive), returning both the
// source file key and a pointer to the entity. Returns zero file and nil if not found.
func (c Collection[T]) GetWithKey(name string) (file.File, *T) {
	for k, item := range c {
		if strings.EqualFold(item.Name(), name) {
			v := c[k]

			return k, &v
		}
	}

	return "", nil
}

// Names returns a sorted list of all entity names.
func (c Collection[T]) Names() []string {
	names := make([]string, 0, len(c))
	for _, item := range c {
		names = append(names, item.Name())
	}

	slices.Sort(names)

	return names
}

// Filter returns a new collection containing only entities matching the predicate.
func (c Collection[T]) Filter(fn func(T) bool) Collection[T] {
	result := make(Collection[T])

	for k, v := range c {
		if fn(v) {
			result[k] = v
		}
	}

	return result
}

// RemoveIf deletes all entries matching the predicate (mutates in-place).
func (c Collection[T]) RemoveIf(fn func(T) bool) {
	for k, v := range c {
		if fn(v) {
			delete(c, k)
		}
	}
}

// Apply calls fn on each entity, writing the result back (mutates in-place).
func (c Collection[T]) Apply(fn func(*T)) {
	for k := range c {
		v := c[k]
		fn(&v)

		c[k] = v
	}
}

// Values returns all entities sorted by Name().
func (c Collection[T]) Values() []T {
	vals := make([]T, 0, len(c))
	for _, v := range c {
		vals = append(vals, v)
	}

	slices.SortFunc(vals, func(a, b T) int {
		return strings.Compare(a.Name(), b.Name())
	})

	return vals
}

// StringCollection is a string-keyed collection of config entities.
// Used for Providers, MCPs, Plugins. Keys are lowercase-normalized at load time.
type StringCollection[T any] map[string]T

// Get retrieves an entity by name (case-insensitive O(1) lookup), returning nil if not found.
func (c StringCollection[T]) Get(name string) *T {
	v, ok := c[strings.ToLower(name)]
	if !ok {
		return nil
	}

	return &v
}

// Names returns a sorted list of all entity keys.
func (c StringCollection[T]) Names() []string {
	return slices.Sorted(maps.Keys(c))
}

// Filter returns a new collection containing only entries matching the predicate.
// The predicate receives both the key and the value.
func (c StringCollection[T]) Filter(fn func(string, T) bool) StringCollection[T] {
	result := make(StringCollection[T])

	for k, v := range c {
		if fn(k, v) {
			result[k] = v
		}
	}

	return result
}

// RemoveIf deletes all entries matching the predicate (mutates in-place).
func (c StringCollection[T]) RemoveIf(fn func(string, T) bool) {
	for k, v := range c {
		if fn(k, v) {
			delete(c, k)
		}
	}
}

// Apply calls fn on each entity, writing the result back (mutates in-place).
func (c StringCollection[T]) Apply(fn func(*T)) {
	for k := range c {
		v := c[k]
		fn(&v)

		c[k] = v
	}
}

// Values returns all entities sorted by key.
func (c StringCollection[T]) Values() []T {
	vals := make([]T, 0, len(c))
	for _, name := range c.Names() {
		vals = append(vals, c[name])
	}

	return vals
}
