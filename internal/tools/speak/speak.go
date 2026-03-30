// Package speak provides a tool for text-to-speech synthesis.
package speak

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/dustin/go-humanize"

	"github.com/idelchi/aura/internal/agent"
	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/providers"
	"github.com/idelchi/aura/pkg/llm/synthesize"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/godyl/pkg/path/file"
)

// Inputs defines the parameters for the Speak tool.
type Inputs struct {
	Text       string `json:"text"            jsonschema:"required,description=Text to convert to speech"                              validate:"required"`
	OutputPath string `json:"output_path"     jsonschema:"required,description=File path to write the audio output (e.g. output.mp3)"  validate:"required"`
	Voice      string `json:"voice,omitempty" jsonschema:"description=Voice identifier (e.g. alloy\\, af_heart). Default from config."`
}

// Tool implements text-to-speech synthesis via a TTS server.
type Tool struct {
	tool.Base

	// Cfg is the full config — needed because this tool creates sub-agents
	// via agent.New(), which requires access to agents, providers, modes,
	// and the full BuildAgent pipeline.
	Cfg      config.Config
	CfgPaths config.Paths
	Rt       *config.Runtime
}

// New creates a Speak tool with the given configuration.
func New(cfg config.Config, paths config.Paths, rt *config.Runtime) *Tool {
	return &Tool{
		Base: tool.Base{
			Text: tool.Text{
				Description: heredoc.Doc(`
					Converts text to speech audio using a TTS server.
					Writes the audio output to the specified file path.
					Returns the output file path on success.
				`),
				Usage: heredoc.Doc(`
					Provide the text to convert and an output file path.
					Optionally provide a voice identifier to override the default.
					The output format is determined by the config (default: mp3).
				`),
				Examples: heredoc.Doc(`
					{"text": "Hello, world!", "output_path": "hello.mp3"}
					{"text": "This is a test.", "output_path": "test.wav", "voice": "af_heart"}
				`),
			},
		},
		Cfg:      cfg,
		CfgPaths: paths,
		Rt:       rt,
	}
}

// Name returns the tool's identifier.
func (t *Tool) Name() string {
	return "Speak"
}

// Schema returns the provider-agnostic tool definition.
func (t *Tool) Schema() tool.Schema {
	return tool.GenerateSchema[Inputs](t)
}

// Available checks if the TTS agent is configured.
func (t *Tool) Available() bool {
	return t.Cfg.Features.TTS.Agent != ""
}

// Sandboxable returns false because the tool makes network calls.
func (t *Tool) Sandboxable() bool {
	return false
}

// Paths returns the filesystem paths this tool call will access.
func (t *Tool) Paths(ctx context.Context, args map[string]any) (read, write []string, err error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return nil, nil, err
	}

	return nil, []string{tool.ResolvePath(ctx, os.ExpandEnv(params.OutputPath))}, nil
}

// Execute synthesizes speech from text and writes the audio to a file.
func (t *Tool) Execute(ctx context.Context, args map[string]any) (string, error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return "", err
	}

	params.OutputPath = tool.ResolvePath(ctx, os.ExpandEnv(params.OutputPath))

	if err := tool.ValidatePath(params.OutputPath); err != nil {
		return "", err
	}

	ttsCfg := t.Cfg.Features.TTS

	ttsAgent, err := agent.New(t.Cfg, t.CfgPaths, t.Rt, ttsCfg.Agent)
	if err != nil {
		return "", fmt.Errorf("creating tts agent %q: %w", ttsCfg.Agent, err)
	}

	// Tool arg overrides config default.
	voice := ttsCfg.Voice
	if params.Voice != "" {
		voice = params.Voice
	}

	synth, ok := providers.As[providers.Synthesizer](ttsAgent.Provider)
	if !ok {
		return "", fmt.Errorf("provider %q does not support speech synthesis", ttsCfg.Agent)
	}

	resp, err := synth.Synthesize(ctx, synthesize.Request{
		Model:  ttsAgent.Model.Name,
		Input:  params.Text,
		Voice:  voice,
		Format: ttsCfg.Format,
		Speed:  ttsCfg.Speed,
	})
	if err != nil {
		return "", fmt.Errorf("speech synthesis: %w", err)
	}

	if len(resp.Audio) == 0 {
		return "", errors.New("speech synthesis returned empty audio")
	}

	if err := file.New(params.OutputPath).Write(resp.Audio); err != nil {
		return "", tool.FileError("writing audio file", params.OutputPath, err)
	}

	return fmt.Sprintf("Audio written to %s (%s)", params.OutputPath, humanize.Bytes(uint64(len(resp.Audio)))), nil
}
