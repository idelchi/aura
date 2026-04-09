package tool

import (
	"context"
	"strings"

	"github.com/idelchi/godyl/pkg/path/file"
)

type workDirKey struct{}

// WithWorkDir returns a derived context carrying the effective working directory.
func WithWorkDir(ctx context.Context, dir string) context.Context {
	return context.WithValue(ctx, workDirKey{}, dir)
}

// WorkDirFromContext extracts the working directory from the context, or "".
func WorkDirFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(workDirKey{}).(string); ok {
		return v
	}

	return ""
}

type sdkContextKey struct{}

// WithSDKContext returns a derived context carrying an SDK context value.
// The value is typed as any because this package cannot import sdk.
func WithSDKContext(ctx context.Context, sc any) context.Context {
	return context.WithValue(ctx, sdkContextKey{}, sc)
}

// SDKContextFromContext extracts the SDK context from the Go context, or nil.
func SDKContextFromContext(ctx context.Context) any {
	return ctx.Value(sdkContextKey{})
}

type streamCallbackKey struct{}

// StreamCallback is called with each complete line of output during streaming execution.
type StreamCallback func(line string)

// WithStreamCallback returns a derived context carrying a streaming callback.
func WithStreamCallback(ctx context.Context, cb StreamCallback) context.Context {
	return context.WithValue(ctx, streamCallbackKey{}, cb)
}

// StreamCallbackFromContext extracts the streaming callback from the context, or nil.
func StreamCallbackFromContext(ctx context.Context) StreamCallback {
	cb, _ := ctx.Value(streamCallbackKey{}).(StreamCallback)

	return cb
}

// ResolvePath converts a relative path to absolute using the context's workdir.
// Absolute paths and empty workdir are returned as-is.
// Leading and trailing whitespace is stripped — LLMs occasionally produce paths
// like " /etc/hostname" which would otherwise be treated as relative.
func ResolvePath(ctx context.Context, path string) string {
	path = strings.TrimSpace(path)

	f := file.New(path)
	if f.IsAbs() {
		return path
	}

	if wd := WorkDirFromContext(ctx); wd != "" {
		return file.New(wd, path).Path()
	}

	return path
}

// ReadBeforePolicy controls which file operations require a prior read.
type ReadBeforePolicy struct {
	Write  bool // require read before overwriting existing files (default: true)
	Delete bool // require read before deleting files (default: false)
}

// DefaultReadBeforePolicy returns the default policy: write enforcement on, delete off.
func DefaultReadBeforePolicy() ReadBeforePolicy {
	return ReadBeforePolicy{Write: true, Delete: false}
}
