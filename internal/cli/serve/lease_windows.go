package serve

import (
	"context"
	"errors"
)

// watchLease is not supported on Windows.
func watchLease(_ int, _ context.CancelCauseFunc) error {
	return errors.New("--launcher-fd is not supported on Windows")
}
