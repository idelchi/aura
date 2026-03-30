package task

import (
	"context"

	"github.com/idelchi/godyl/pkg/env"
)

type envContextKey struct{}

// WithEnv returns a context carrying task-scoped environment variables.
func WithEnv(ctx context.Context, e env.Env) context.Context {
	return context.WithValue(ctx, envContextKey{}, e)
}

// EnvFromContext returns task-scoped environment variables, or nil if none set.
func EnvFromContext(ctx context.Context) env.Env {
	e, _ := ctx.Value(envContextKey{}).(env.Env)

	return e
}
