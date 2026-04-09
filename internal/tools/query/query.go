// Package query provides a tool for embedding-based code search.
package query

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/internal/agent"
	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/indexer"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// Inputs defines the parameters for the Query tool.
type Inputs struct {
	// Query is the search query for embedding-based similarity.
	Query string `json:"query" jsonschema:"required,description=Search query for embedding-based similarity" validate:"required"`
	// K is the number of results to return (default: from config).
	K int `json:"k,omitempty" jsonschema:"description=Number of results to return (default from config)"`
	// FullContent returns chunk content in results when true.
	FullContent bool `json:"full_content,omitempty" jsonschema:"description=Return chunk content in results (default false)"`
	// Reranking enables the reranking pass. Nil/omitted = true (default on).
	Reranking *bool `json:"reranking,omitempty" jsonschema:"description=Include reranking pass (default true)"`
}

// Result represents a single search result for JSON output.
type Result struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Content   string `json:"content,omitempty"`
}

// Tool implements embedding-based code search.
type Tool struct {
	tool.Base

	// Cfg is the full config — needed because this tool creates sub-agents
	// via agent.New(), which requires access to agents, providers, modes,
	// and the full BuildAgent pipeline. Also uses Home (embedding cache path)
	// and Features.Estimation (token estimator).
	Cfg    config.Config
	Paths  config.Paths
	Rt     *config.Runtime
	Events chan<- ui.Event
}

// New creates a Query tool with the given configuration.
func New(cfg config.Config, paths config.Paths, rt *config.Runtime) *Tool {
	return &Tool{
		Base: tool.Base{
			Text: tool.Text{
				Description: heredoc.Doc(`
					Embedding-based CHUNK-LEVEL search across the CURRENT codebase using natural language.
					Uses token-aware chunking with embedding similarity and optional reranking.
					Results are deduplicated by file path.

					Best for: component/module names, architectural concepts, feature areas, function-level search.
				`),
				Usage: heredoc.Doc(`
					ONLY use this tool to explore the CURRENT codebase, not for searching generic or external information.

					Query with component names or purposes.

					Good: "token counter", "file reading tool", "ollama client", "remove messages from conversation"
					Bad: "how does X work"

					Use k=3-5 for targeted searches, k=10 for exploration.
					Use full_content=true only when you need chunk contents.
				`),
				Examples: heredoc.Doc(`
					{"query": "ollama client implementation", "k": 5}
					{"query": "token counting", "k": 3}
					{"query": "configuration loading", "k": 10}
					{"query": "file reading tool", "k": 3, "full_content": true}
				`),
			},
		},
		Cfg:   cfg,
		Paths: paths,
		Rt:    rt,
	}
}

// Name returns the tool's identifier.
func (t *Tool) Name() string {
	return "Query"
}

// Schema returns the provider-agnostic tool definition.
func (t *Tool) Schema() tool.Schema {
	return tool.GenerateSchema[Inputs](t)
}

// Available checks if the embeddings agent is configured.
func (t *Tool) Available() bool {
	return t.Cfg.Features.Embeddings.Agent != ""
}

// Sandboxable returns false because the tool accesses the filesystem and network.
func (t *Tool) Sandboxable() bool {
	return false
}

// Execute indexes the codebase and searches for similar content.
func (t *Tool) Execute(ctx context.Context, args map[string]any) (string, error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return "", err
	}

	searchCfg := t.Cfg.Features.Embeddings

	searchAgent, err := agent.New(t.Cfg, t.Paths, t.Rt, searchCfg.Agent)
	if err != nil {
		return "", fmt.Errorf("creating search agent %q: %w", searchCfg.Agent, err)
	}

	estimator, err := t.Cfg.Features.Estimation.NewEstimator()
	if err != nil {
		return "", fmt.Errorf("creating estimator: %w", err)
	}

	chunkCfg := searchCfg.Chunking.ChunkerConfig(estimator.EstimateLocal)

	gi := indexer.BuildGitignore(searchCfg.Gitignore)

	idx, err := indexer.New(
		searchAgent.Provider,
		searchAgent.Model.Name,
		folder.New(t.Paths.Home, "embeddings").Path(),
		chunkCfg,
		gi,
	)
	if err != nil {
		return "", fmt.Errorf("creating indexer: %w", err)
	}

	if t.Events != nil {
		idx.OnProgress = func(phase string) {
			t.Events <- ui.SpinnerMessage{Text: phase}
		}
	}

	// Set up reranker if configured and not explicitly disabled
	if searchCfg.Reranking.Agent != "" && (params.Reranking == nil || *params.Reranking) {
		rerankAgent, err := agent.New(t.Cfg, t.Paths, t.Rt, searchCfg.Reranking.Agent)
		if err != nil {
			return "", fmt.Errorf("creating rerank agent %q: %w", searchCfg.Reranking.Agent, err)
		}

		idx.UseReranker(rerankAgent.Provider, rerankAgent.Model.Name)
	}

	idx.SetOffload(searchCfg.Offload, searchCfg.Reranking.Offload)

	if _, err := idx.Index(ctx); err != nil {
		return "", fmt.Errorf("indexing: %w", err)
	}

	k := params.K
	if k <= 0 {
		k = searchCfg.MaxResults
	}

	// Query returns both unsorted and reranked — tool uses reranked (best results)
	_, reranked, err := idx.Query(ctx, params.Query, k, searchCfg.Reranking.Multiplier)
	if err != nil {
		return "", fmt.Errorf("querying: %w", err)
	}

	if len(reranked) == 0 {
		return "No matches found", nil
	}

	output := make([]Result, len(reranked))
	for i, r := range reranked {
		out := Result{
			Path:      r.Path,
			StartLine: r.StartLine,
			EndLine:   r.EndLine,
		}

		if params.FullContent {
			out.Content = r.Content
		}

		output[i] = out
	}

	jsonBytes, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling results: %w", err)
	}

	return string(jsonBytes), nil
}
