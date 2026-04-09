// Package auth manages persistent token storage for OAuth-based providers.
package auth

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// Load searches directories in order for a token file named provider.
// Returns the token content and the path it was loaded from.
// Returns os.ErrNotExist if no token is found in any directory.
func Load(provider string, dirs ...string) (string, string, error) {
	for _, dir := range dirs {
		if dir == "" {
			continue
		}

		f := folder.New(dir).WithFile(provider)

		data, err := f.Read()
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}

			return "", "", err
		}

		token := strings.TrimSpace(string(data))
		if token == "" {
			continue
		}

		return token, f.Path(), nil
	}

	return "", "", fmt.Errorf("no stored token for %q: %w", provider, os.ErrNotExist)
}

// Save writes a token to dir/provider. Creates the directory if needed.
func Save(dir, provider, token string) error {
	if err := folder.New(dir).Create(0o700); err != nil {
		return fmt.Errorf("creating auth directory: %w", err)
	}

	return file.New(dir, provider).Write([]byte(token + "\n"))
}

// Path returns dir/provider.
func Path(dir, provider string) string {
	return file.New(dir, provider).Path()
}
