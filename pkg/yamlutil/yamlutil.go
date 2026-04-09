package yamlutil

import (
	"errors"

	"go.yaml.in/yaml/v4"
)

// StrictUnmarshal is a drop-in replacement for yaml.Unmarshal that rejects unknown fields.
// Empty and comment-only content is handled gracefully (returns nil, like yaml.Unmarshal).
func StrictUnmarshal(data []byte, v any) error {
	return filterNoDocuments(yaml.Load(data, v, yaml.WithKnownFields()))
}

// filterNoDocuments removes "no documents in stream" errors from LoadErrors.
// These occur for empty or comment-only YAML files, which yaml.Unmarshal handles silently.
func filterNoDocuments(err error) error {
	if err == nil {
		return nil
	}

	var le *yaml.LoadErrors
	if !errors.As(err, &le) {
		return err
	}

	var real []*yaml.LoadError

	for _, ce := range le.Errors {
		if ce.Err.Error() != "yaml: no documents in stream" {
			real = append(real, ce)
		}
	}

	if len(real) == 0 {
		return nil
	}

	return &yaml.LoadErrors{Errors: real}
}
