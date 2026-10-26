package atomicfile

import (
	"bytes"

	"github.com/natefinch/atomic"
)

// WriteFile syncs a same-directory temporary file and publishes it with MoveFileEx's
// replace-existing and write-through flags. Windows uses directory ACLs, not POSIX modes;
// directory fsync is not a supported operation on a Windows directory handle.
func WriteFile(path string, data []byte) error {
	return atomic.WriteFile(path, bytes.NewReader(data))
}
