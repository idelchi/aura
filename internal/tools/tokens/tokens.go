// Package tokens provides a tool for counting tokens in files or inline content.
package tokens

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/internal/agent"
	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/tool"
	pkgproviders "github.com/idelchi/aura/pkg/providers"
)

// validMethods defines the accepted values for the method parameter.
var validMethods = map[string]bool{
	"rough":          true,
	"tiktoken":       true,
	"rough+tiktoken": true,
	"native":         true,
}

// Inputs defines the parameters for the Tokens tool.
type Inputs struct {
	Path    string `json:"path,omitempty"    jsonschema:"description=File path to count tokens in"`
	Content string `json:"content,omitempty" jsonschema:"description=Inline text to count tokens in"`
	Method  string `json:"method,omitempty"  jsonschema:"description=Estimation method: rough\\, tiktoken\\, rough+tiktoken\\, native (default: configured method)"`
}

// Tool counts tokens in files or inline content.
// Follows the Vision/Transcribe/Speak pattern: stores (cfg, paths, rt) and
// reads the session estimator from rt.Estimator.
type Tool struct {
	tool.Base

	Cfg      config.Config
	CfgPaths config.Paths
	Rt       *config.Runtime
}

// New creates a Tokens tool with the given configuration.
func New(cfg config.Config, paths config.Paths, rt *config.Runtime) *Tool {
	return &Tool{
		Base: tool.Base{
			Text: tool.Text{
				Description: `Count tokens in a file or inline content using the configured estimation method`,
				Usage: heredoc.Doc(`
					Provide either a file path or inline content to count tokens.
					Optionally override the estimation method.
				`),
				Examples: heredoc.Doc(`
					{"path": "go.mod"}
					{"content": "The quick brown fox jumps over the lazy dog"}
					{"path": "README.md", "method": "rough"}
					{"path": "main.go", "method": "tiktoken"}
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
	return "Tokens"
}

// Schema returns the provider-agnostic tool definition.
func (t *Tool) Schema() tool.Schema {
	return tool.GenerateSchema[Inputs](t)
}

// Sandboxable returns false because native estimation makes network calls to a provider.
func (t *Tool) Sandboxable() bool {
	return false
}

// Paths declares read access for sandbox enforcement.
func (t *Tool) Paths(ctx context.Context, args map[string]any) (read, write []string, err error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return nil, nil, err
	}

	if params.Path != "" {
		return []string{tool.ResolvePath(ctx, os.ExpandEnv(params.Path))}, nil, nil
	}

	return nil, nil, nil
}

// Execute counts tokens and returns the count as a plain number string.
func (t *Tool) Execute(ctx context.Context, args map[string]any) (string, error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return "", err
	}

	if params.Path != "" && params.Content != "" {
		return "", errors.New("provide either path or content, not both")
	}

	if params.Path == "" && params.Content == "" {
		return "", errors.New("provide either path or content")
	}

	if params.Method != "" && !validMethods[params.Method] {
		return "", fmt.Errorf("invalid method %q (valid: rough, tiktoken, rough+tiktoken, native)", params.Method)
	}

	// Resolve content from file or inline.
	content := params.Content
	if params.Path != "" {
		path := tool.ResolvePath(ctx, os.ExpandEnv(params.Path))

		if err := tool.ValidatePath(path); err != nil {
			return "", err
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return "", tool.FileError("reading", path, err)
		}

		content = string(data)
	}

	if strings.TrimSpace(content) == "" {
		return "0", nil
	}

	count, err := t.estimate(ctx, params.Method, content)
	if err != nil {
		return "", err
	}

	return strconv.Itoa(count), nil
}

// estimate returns the token count using the specified method (or the configured default).
func (t *Tool) estimate(ctx context.Context, method, content string) (int, error) {
	switch method {
	case "rough":
		return t.local(content, method)
	case "tiktoken":
		return t.local(content, method)
	case "rough+tiktoken":
		return t.local(content, method)
	case "native":
		return t.estimateNative(ctx, content)
	default:
		// Empty method: use session estimator if available (respects agent overrides
		// and UseNative wiring). Otherwise fall back to local from config.
		if est := t.Rt.Estimator; est != nil {
			return est.Estimate(ctx, content), nil
		}

		return t.local(content, "")
	}
}

// local creates an estimator from config and returns the count for a local method.
func (t *Tool) local(content, method string) (int, error) {
	est, err := t.Cfg.Features.Estimation.NewEstimator()
	if err != nil {
		return 0, err
	}

	switch method {
	case "rough":
		return est.Rough(content), nil
	case "tiktoken":
		return est.Tiktoken(content), nil
	case "rough+tiktoken":
		return max(est.Rough(content), est.Tiktoken(content)), nil
	default:
		return est.EstimateLocal(content), nil
	}
}

// estimateNative handles native token estimation.
// In-session: rt.Estimator has UseNative wired by the assistant — use it.
// Standalone: resolve the default agent from config and call provider.Estimate() directly.
func (t *Tool) estimateNative(ctx context.Context, content string) (int, error) {
	// Session path: estimator has native wired by the assistant.
	if est := t.Rt.Estimator; est != nil && est.HasNative() {
		return est.Estimate(ctx, content), nil
	}

	// Standalone path: resolve agent from config and call provider directly.
	resolved, err := config.ResolveDefault(t.Cfg.Agents, append([]string{t.CfgPaths.Global}, t.CfgPaths.Home))
	if err != nil {
		return 0, fmt.Errorf("resolving agent for native estimation: %w", err)
	}

	ag, err := agent.New(t.Cfg, t.CfgPaths, t.Rt, resolved.Metadata.Name)
	if err != nil {
		return 0, fmt.Errorf("creating agent for native estimation: %w", err)
	}

	mdl, err := ag.Provider.Model(ctx, ag.Model.Name)
	if err != nil {
		return 0, fmt.Errorf("resolving model %q: %w", ag.Model.Name, err)
	}

	req := request.Request{
		Model:         mdl,
		ContextLength: ag.Model.Context,
	}

	count, err := ag.Provider.Estimate(ctx, req, content)
	if err != nil {
		if errors.Is(err, pkgproviders.ErrContextExhausted) {
			return count, nil
		}

		return 0, fmt.Errorf("native estimation: %w", err)
	}

	return count, nil
}
