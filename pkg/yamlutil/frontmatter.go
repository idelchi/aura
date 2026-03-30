package yamlutil

import "github.com/adrg/frontmatter"

// StrictYAMLFormats returns frontmatter format handlers that use yaml.v4
// with strict field checking instead of the default yaml.v2 unmarshal.
func StrictYAMLFormats() []*frontmatter.Format {
	return []*frontmatter.Format{
		frontmatter.NewFormat("---", "---", StrictUnmarshal),
		frontmatter.NewFormat("---yaml", "---", StrictUnmarshal),
	}
}
