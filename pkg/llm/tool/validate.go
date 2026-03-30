package tool

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
)

// ValidateInput converts a map to a typed struct and validates it.
// Rejects unknown fields and enriches errors with valid parameter names.
func ValidateInput[T any](args map[string]any, schema Schema) (*T, error) {
	data, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("marshaling args: %w", err)
	}

	var params T

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&params); err != nil {
		if strings.Contains(err.Error(), "unknown field") {
			return nil, fmt.Errorf("%w\n\nValid parameters: %s", err,
				strings.Join(schema.ParamNames(), ", "))
		}

		return nil, fmt.Errorf("unmarshaling args: %w", err)
	}

	validate := validator.New()
	if err := validate.Struct(params); err != nil {
		return nil, fmt.Errorf("validating args: %w", err)
	}

	return &params, nil
}

// ValidatePath rejects root path operations.
func ValidatePath(path string) error {
	if strings.TrimSpace(path) == "/" {
		return errors.New("path cannot be root '/'")
	}

	return nil
}
