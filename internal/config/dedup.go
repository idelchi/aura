package config

import (
	"strings"

	"github.com/idelchi/godyl/pkg/path/file"
)

// dedupByName removes any existing entry whose name matches case-insensitively
// but differs in casing. Later files override earlier ones.
func dedupByName[M any](name string, metas map[string]M, bodies map[string]string, fileMap map[string]file.File) {
	for existing := range metas {
		if strings.EqualFold(existing, name) && existing != name {
			delete(metas, existing)
			delete(bodies, existing)
			delete(fileMap, existing)

			break
		}
	}
}
