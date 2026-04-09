package ollama

import (
	"context"
	"strings"

	"github.com/ollama/ollama/api"

	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/providers"
)

// Estimate tokenizes content using Ollama's /api/chat endpoint.
// Wraps the content as a single user message so the token count includes chat template
// overhead (BOS, role markers, delimiters) — matching what the real chat path consumes.
// Returns prompt_eval_count on success, or numCtx on overflow (400 "input length exceeds").
func (c *Client) Estimate(ctx context.Context, req request.Request, content string) (int, error) {
	numCtx := req.ContextLength
	if numCtx == 0 {
		numCtx = int(req.Model.ContextLength)
	}

	streamOff := false
	truncateOff := false
	shiftOff := false

	chatReq := &api.ChatRequest{
		Model: req.Model.Name,
		Messages: []api.Message{
			{Role: "user", Content: content},
		},
		Stream:   &streamOff,
		Truncate: &truncateOff,
		Shift:    &shiftOff,
		Options: map[string]any{
			"num_predict": 1,
			"num_ctx":     numCtx,
		},
	}

	if c.keepAlive > 0 {
		chatReq.KeepAlive = &api.Duration{Duration: c.keepAlive}
	}

	var result int

	err := c.Client.Chat(ctx, chatReq, func(resp api.ChatResponse) error {
		result = resp.PromptEvalCount

		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "input length exceeds") {
			return numCtx, providers.ErrContextExhausted
		}

		return 0, err
	}

	return result, nil
}
