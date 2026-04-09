package providers

import (
	"context"

	"github.com/idelchi/aura/pkg/llm/embedding"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/rerank"
	"github.com/idelchi/aura/pkg/llm/stream"
	"github.com/idelchi/aura/pkg/llm/synthesize"
	"github.com/idelchi/aura/pkg/llm/transcribe"
	"github.com/idelchi/aura/pkg/llm/usage"
)

// Provider defines the core interface for LLM provider implementations.
type Provider interface {
	Chat(context.Context, request.Request, stream.Func) (message.Message, usage.Usage, error)
	Models(context.Context) (model.Models, error)
	Model(context.Context, string) (model.Model, error)
	Estimate(context.Context, request.Request, string) (int, error)
}

// Embedder is an optional interface for providers that support embedding.
type Embedder interface {
	Embed(context.Context, embedding.Request) (embedding.Response, usage.Usage, error)
}

// Reranker is an optional interface for providers that support reranking.
type Reranker interface {
	Rerank(context.Context, rerank.Request) (rerank.Response, error)
}

// Transcriber is an optional interface for providers that support audio transcription.
type Transcriber interface {
	Transcribe(context.Context, transcribe.Request) (transcribe.Response, error)
}

// Synthesizer is an optional interface for providers that support speech synthesis.
type Synthesizer interface {
	Synthesize(context.Context, synthesize.Request) (synthesize.Response, error)
}

// ModelLoader is an optional interface for providers that support explicit model load/unload.
type ModelLoader interface {
	LoadModel(ctx context.Context, name string) error
	UnloadModel(ctx context.Context, name string) error
}

// As unwraps wrapper layers (RetryProvider, etc.) to find the first
// provider that satisfies interface T. Follows the same pattern as errors.As.
func As[T any](p Provider) (T, bool) {
	for {
		if t, ok := p.(T); ok {
			return t, true
		}

		u, ok := p.(interface{ Unwrap() Provider })
		if !ok {
			var zero T

			return zero, false
		}

		p = u.Unwrap()
	}
}
