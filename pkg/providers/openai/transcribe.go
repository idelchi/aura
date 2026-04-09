package openai

import (
	"context"
	"fmt"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"

	"github.com/idelchi/aura/pkg/llm/transcribe"
	"github.com/idelchi/godyl/pkg/path/file"
)

// Transcribe sends an audio file to the server for speech-to-text transcription.
func (c *Client) Transcribe(ctx context.Context, req transcribe.Request) (transcribe.Response, error) {
	f, err := file.New(req.FilePath).Open()
	if err != nil {
		return transcribe.Response{}, fmt.Errorf("opening audio file: %w", err)
	}
	defer f.Close()

	params := openai.AudioTranscriptionNewParams{
		File:  f,
		Model: openai.AudioModel(req.Model),
	}

	if req.Language != "" {
		params.Language = param.NewOpt(req.Language)
	}

	resp, err := c.Audio.Transcriptions.New(ctx, params)
	if err != nil {
		return transcribe.Response{}, fmt.Errorf("transcription: %w", err)
	}

	return transcribe.Response{
		Text: resp.Text,
	}, nil
}
