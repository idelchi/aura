// Package transcribe provides a tool for speech-to-text transcription.
package transcribe

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/internal/agent"
	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/providers"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/aura/pkg/llm/transcribe"
	"github.com/idelchi/godyl/pkg/path/file"
)

// Inputs defines the parameters for the Transcribe tool.
type Inputs struct {
	Path     string `json:"path"               jsonschema:"required,description=Path to the audio file (mp3, wav, ogg, flac, m4a, webm)"         validate:"required"`
	Language string `json:"language,omitempty" jsonschema:"description=ISO-639-1 language code hint (e.g. en\\, de\\, ja). Empty = auto-detect."`
}

// Tool implements speech-to-text transcription via a whisper-compatible server.
type Tool struct {
	tool.Base

	// Cfg is the full config — needed because this tool creates sub-agents
	// via agent.New(), which requires access to agents, providers, modes,
	// and the full BuildAgent pipeline.
	Cfg      config.Config
	CfgPaths config.Paths
	Rt       *config.Runtime
}

// New creates a Transcribe tool with the given configuration.
func New(cfg config.Config, paths config.Paths, rt *config.Runtime) *Tool {
	return &Tool{
		Base: tool.Base{
			Text: tool.Text{
				Description: heredoc.Doc(`
					Transcribes an audio file to text using a speech-to-text server.
					Supports common audio formats: mp3, wav, ogg, flac, m4a, webm.
					Returns the transcribed text content.
				`),
				Usage: heredoc.Doc(`
					Provide a file path to an audio file.
					Optionally provide a language hint in ISO-639-1 format to improve accuracy.
					If no language is given, the server auto-detects the language.
				`),
				Examples: heredoc.Doc(`
					{"path": "recording.mp3"}
					{"path": "meeting.wav", "language": "en"}
					{"path": "notes.ogg", "language": "ja"}
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
	return "Transcribe"
}

// Schema returns the provider-agnostic tool definition.
func (t *Tool) Schema() tool.Schema {
	return tool.GenerateSchema[Inputs](t)
}

// Available checks if the STT agent is configured.
func (t *Tool) Available() bool {
	return t.Cfg.Features.STT.Agent != ""
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

	return []string{tool.ResolvePath(ctx, os.ExpandEnv(params.Path))}, nil, nil
}

// Execute transcribes an audio file and returns the text.
func (t *Tool) Execute(ctx context.Context, args map[string]any) (string, error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return "", err
	}

	params.Path = tool.ResolvePath(ctx, os.ExpandEnv(params.Path))

	if err := tool.ValidatePath(params.Path); err != nil {
		return "", err
	}

	sttCfg := t.Cfg.Features.STT

	if !file.New(params.Path).Exists() {
		return "", fmt.Errorf("audio file not found: %s", params.Path)
	}

	sttAgent, err := agent.New(t.Cfg, t.CfgPaths, t.Rt, sttCfg.Agent)
	if err != nil {
		return "", fmt.Errorf("creating stt agent %q: %w", sttCfg.Agent, err)
	}

	// Tool arg overrides config default.
	language := sttCfg.Language
	if params.Language != "" {
		language = params.Language
	}

	transcriber, ok := providers.As[providers.Transcriber](sttAgent.Provider)
	if !ok {
		return "", fmt.Errorf("provider %q does not support transcription", sttCfg.Agent)
	}

	resp, err := transcriber.Transcribe(ctx, transcribe.Request{
		Model:    sttAgent.Model.Name,
		FilePath: params.Path,
		Language: language,
	})
	if err != nil {
		return "", fmt.Errorf("transcription: %w", err)
	}

	text := strings.TrimSpace(resp.Text)
	if text == "" {
		return "", errors.New("transcription returned empty text")
	}

	return text, nil
}
