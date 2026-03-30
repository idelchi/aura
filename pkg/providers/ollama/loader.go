package ollama

import (
	"context"

	"github.com/ollama/ollama/api"
)

// LoadModel explicitly loads a model into memory on the Ollama server.
// Uses the generate endpoint with an empty prompt, which triggers a load-only operation.
func (c *Client) LoadModel(ctx context.Context, name string) error {
	req := &api.GenerateRequest{
		Model: name,
	}

	if c.keepAlive > 0 {
		req.KeepAlive = &api.Duration{Duration: c.keepAlive}
	}

	return handleError(c.Generate(ctx, req, func(api.GenerateResponse) error { return nil }))
}

// UnloadModel explicitly unloads a model from memory on the Ollama server.
// Uses the generate endpoint with KeepAlive set to zero, which triggers immediate unload.
func (c *Client) UnloadModel(ctx context.Context, name string) error {
	return handleError(c.Generate(ctx, &api.GenerateRequest{
		Model:     name,
		KeepAlive: &api.Duration{Duration: 0},
	}, func(api.GenerateResponse) error { return nil }))
}
