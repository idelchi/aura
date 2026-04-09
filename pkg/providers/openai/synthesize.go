package openai

import (
	"context"
	"fmt"
	"io"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"

	"github.com/idelchi/aura/pkg/llm/synthesize"
)

// Synthesize converts text to speech audio via the TTS endpoint.
func (c *Client) Synthesize(ctx context.Context, req synthesize.Request) (synthesize.Response, error) {
	params := openai.AudioSpeechNewParams{
		Input: req.Input,
		Model: openai.SpeechModel(req.Model),
		Voice: openai.AudioSpeechNewParamsVoiceUnion{OfString: param.NewOpt(req.Voice)},
	}

	if req.Format != "" {
		params.ResponseFormat = openai.AudioSpeechNewParamsResponseFormat(req.Format)
	}

	if req.Speed != 0 {
		params.Speed = param.NewOpt(req.Speed)
	}

	resp, err := c.Audio.Speech.New(ctx, params)
	if err != nil {
		return synthesize.Response{}, fmt.Errorf("speech synthesis: %w", err)
	}
	defer resp.Body.Close()

	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return synthesize.Response{}, fmt.Errorf("reading audio response: %w", err)
	}

	return synthesize.Response{
		Audio: audio,
	}, nil
}
