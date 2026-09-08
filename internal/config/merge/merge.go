// Package merge provides the canonical struct merge function for config inheritance and overlay.
//
// All config merging in the project MUST use [Merge] instead of calling mergo directly.
// This ensures consistent semantics: nil = inherit, non-nil (including empty) = override.
package merge

import (
	"reflect"

	"dario.cat/mergo"
)

// overrideTransformer distinguishes nil from non-nil for pointers, slices and maps.
// nil = inherit from parent, non-nil (including empty) = override parent.
// It also replaces pointers to structs, which WithoutDereference alone leaves unchanged.
type overrideTransformer struct{}

func (overrideTransformer) Transformer(typ reflect.Type) func(dst, src reflect.Value) error {
	switch typ.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Map:
		return func(dst, src reflect.Value) error {
			if src.IsNil() {
				return nil
			}

			dst.Set(src)

			return nil
		}
	}

	return nil
}

// Merge applies src on top of dst using the project's canonical merge semantics:
//   - Non-zero src values replace dst values (WithOverride)
//   - Pointer fields: nil = inherit, non-nil = replace the entire value (including structs)
//   - Slices and maps: nil = inherit, empty = clear parent (overrideTransformer)
func Merge(dst, src any) error {
	return mergo.Merge(dst, src,
		mergo.WithOverride,
		mergo.WithoutDereference,
		mergo.WithTransformers(overrideTransformer{}),
	)
}
