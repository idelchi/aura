// Package vision provides a tool for LLM-based image analysis and text extraction.
package vision

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/internal/agent"
	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/pkg/image"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/stream"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/godyl/pkg/path/file"
)

// Inputs defines the parameters for the Vision tool.
type Inputs struct {
	// Path is the file path to the image or PDF.
	Path string `json:"path" jsonschema:"required,description=Path to the image or PDF file" validate:"required"`
	// Instruction describes what to extract or analyze.
	Instruction string `json:"instruction,omitempty" jsonschema:"description=What to extract or analyze (default: describe or extract text)"`
}

// Tool implements vision-based image analysis via an LLM.
type Tool struct {
	tool.Base

	// Cfg is the full config — needed because this tool creates sub-agents
	// via agent.New(), which requires access to agents, providers, modes,
	// and the full BuildAgent pipeline.
	Cfg      config.Config
	CfgPaths config.Paths
	Rt       *config.Runtime
}

// New creates a Vision tool with the given configuration.
func New(cfg config.Config, paths config.Paths, rt *config.Runtime) *Tool {
	return &Tool{
		Base: tool.Base{
			Text: tool.Text{
				Description: heredoc.Doc(`
					Reads an image or PDF file and sends it to a vision-capable LLM for analysis.
					Returns the LLM's text response describing or extracting content from the image.
				`),
				Usage: heredoc.Doc(`
					Provide a file path to an image (PNG, JPG, GIF) or PDF.
					Optionally provide an instruction describing what to extract or analyze.
					If no instruction is given, the tool extracts text or describes visual content.
				`),
				Examples: heredoc.Doc(`
					{"path": "screenshot.png"}
					{"path": "diagram.jpg", "instruction": "Describe the architecture shown in this diagram"}
					{"path": "document.pdf", "instruction": "Extract all text from this document"}
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
	return "Vision"
}

// Schema returns the provider-agnostic tool definition.
func (t *Tool) Schema() tool.Schema {
	return tool.GenerateSchema[Inputs](t)
}

// Available checks if the vision agent is configured.
func (t *Tool) Available() bool {
	return t.Cfg.Features.Vision.Agent != ""
}

// Sandboxable returns false because the tool makes network calls to an LLM provider.
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

// Execute reads an image/PDF, compresses it, sends it to a vision LLM, and returns the response.
func (t *Tool) Execute(ctx context.Context, args map[string]any) (string, error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return "", err
	}

	params.Path = tool.ResolvePath(ctx, os.ExpandEnv(params.Path))

	if err := tool.ValidatePath(params.Path); err != nil {
		return "", err
	}

	instruction := params.Instruction
	if instruction == "" {
		instruction = "Analyze this image. If it contains text, extract and return the text content. Otherwise, describe what you see."
	}

	visionCfg := t.Cfg.Features.Vision

	visionAgent, err := agent.New(t.Cfg, t.CfgPaths, t.Rt, visionCfg.Agent)
	if err != nil {
		return "", fmt.Errorf("creating vision agent %q: %w", visionCfg.Agent, err)
	}

	imgs, err := loadImages(params.Path, visionCfg.Dimension, visionCfg.Quality)
	if err != nil {
		return "", fmt.Errorf("loading image: %w", err)
	}

	// Build message images — always raw bytes; providers handle encoding
	msgImages := make(message.Images, len(imgs))
	for i, img := range imgs {
		msgImages[i] = message.Image{Data: img}
	}

	mdl, err := visionAgent.Provider.Model(ctx, visionAgent.Model.Name)
	if err != nil {
		return "", fmt.Errorf("resolving vision model: %w", err)
	}

	req := request.Request{
		Model: mdl,
		Messages: message.New(
			message.Message{
				Role:    roles.User,
				Content: instruction,
				Images:  msgImages,
			},
		),
		ContextLength: visionAgent.Model.Context,
	}

	noopStream := stream.Func(func(_, _ string, _ bool) error { return nil })

	response, _, err := visionAgent.Provider.Chat(ctx, req, noopStream)
	if err != nil {
		return "", fmt.Errorf("vision chat: %w", err)
	}

	content := strings.TrimSpace(response.Content)
	if content == "" {
		return "", errors.New("vision model returned empty response")
	}

	return content, nil
}

// loadImages reads the file at path and returns compressed image bytes.
// For PDFs, returns one image per page. For regular images, returns a single-element slice.
func loadImages(path string, dimension, quality int) (image.Images, error) {
	ext := file.New(path).Extension()

	if ext == "pdf" {
		imgs, err := image.FromPDF(path)
		if err != nil {
			return nil, err
		}

		return imgs.Compress(dimension, quality)
	}

	img, err := image.New(path)
	if err != nil {
		return nil, err
	}

	compressed, err := img.Compress(dimension, quality)
	if err != nil {
		return nil, err
	}

	return image.Images{compressed}, nil
}
