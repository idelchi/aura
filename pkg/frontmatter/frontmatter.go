package frontmatter

import (
	"bytes"

	"github.com/adrg/frontmatter"

	"github.com/idelchi/aura/pkg/yamlutil"
	"github.com/idelchi/godyl/pkg/path/file"
)

// Load parses frontmatter from the file into data and returns the remaining content.
func Load(file file.File, data any) (string, error) {
	content, err := file.Read()
	if err != nil {
		return "", err
	}

	body, err := frontmatter.Parse(bytes.NewReader(content), data, yamlutil.StrictYAMLFormats()...)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// LoadRaw reads a frontmatter file and returns the raw YAML bytes and body
// without decoding into a struct. Used by the inheritance pipeline which
// needs yaml.Node-level access before struct decode.
func LoadRaw(file file.File) (yamlBytes []byte, body string, err error) {
	content, err := file.Read()
	if err != nil {
		return nil, "", err
	}

	var captured []byte

	capture := func(data []byte, _ any) error {
		captured = make([]byte, len(data))
		copy(captured, data)

		return nil
	}

	rest, err := frontmatter.Parse(
		bytes.NewReader(content), nil,
		frontmatter.NewFormat("---", "---", capture),
		frontmatter.NewFormat("---yaml", "---", capture),
	)
	if err != nil {
		return nil, "", err
	}

	return captured, string(rest), nil
}
