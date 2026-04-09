package directive

import (
	"os"
	"regexp"
)

var pathPattern = regexp.MustCompile(`@Path\[([^\]]+)\]`)

// parsePaths processes all @Path[path] directives in the input.
// Each match is replaced with the bare path (env vars expanded).
func parsePaths(input string) string {
	return pathPattern.ReplaceAllStringFunc(input, func(match string) string {
		subs := pathPattern.FindStringSubmatch(match)

		return os.ExpandEnv(subs[1])
	})
}
