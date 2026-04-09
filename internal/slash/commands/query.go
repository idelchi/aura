package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/idelchi/aura/internal/agent"
	"github.com/idelchi/aura/internal/indexer"
	"github.com/idelchi/aura/internal/slash"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// Query creates the /query command for embedding-based codebase search.
func Query() slash.Command {
	return slash.Command{
		Name:        "/query",
		Description: "Embedding-based search across the codebase",
		Hints:       "[search terms]",
		Category:    "search",
		Execute: func(ctx context.Context, c slash.Context, args ...string) (string, error) {
			cfg := c.Cfg()
			searchCfg := cfg.Features.Embeddings

			if searchCfg.Agent == "" {
				return "", errors.New("embeddings requires an agent configured in features/embeddings.yaml")
			}

			searchAgent, err := agent.New(cfg, c.Paths(), c.Runtime(), searchCfg.Agent)
			if err != nil {
				return "", fmt.Errorf("creating search agent: %w", err)
			}

			estimator, err := cfg.Features.Estimation.NewEstimator()
			if err != nil {
				return "", fmt.Errorf("creating estimator: %w", err)
			}

			chunkCfg := searchCfg.Chunking.ChunkerConfig(estimator.EstimateLocal)

			gi := indexer.BuildGitignore(searchCfg.Gitignore)

			idx, err := indexer.New(
				searchAgent.Provider,
				searchAgent.Model.Name,
				folder.New(c.Paths().Home, "embeddings").Path(),
				chunkCfg,
				gi,
			)
			if err != nil {
				return "", fmt.Errorf("creating indexer: %w", err)
			}

			idx.OnProgress = func(phase string) {
				c.EventChan() <- ui.SpinnerMessage{Text: phase}
			}

			hasReranker := searchCfg.Reranking.Agent != ""
			if hasReranker {
				rerankAgent, err := agent.New(cfg, c.Paths(), c.Runtime(), searchCfg.Reranking.Agent)
				if err != nil {
					return "", fmt.Errorf("creating rerank agent: %w", err)
				}

				idx.UseReranker(rerankAgent.Provider, rerankAgent.Model.Name)
			}

			idx.SetOffload(searchCfg.Offload, searchCfg.Reranking.Offload)

			stats, err := idx.Index(ctx)
			if err != nil {
				return "", fmt.Errorf("indexing: %w", err)
			}

			// No args = reindex only
			if len(args) == 0 {
				return "Reindexed: " + stats.Display(), nil
			}

			query := strings.Join(args, " ")

			unsorted, reranked, err := idx.Query(ctx, query, searchCfg.MaxResults, searchCfg.Reranking.Multiplier)
			if err != nil {
				return "", fmt.Errorf("querying: %w", err)
			}

			return unsorted.DisplayWithReranked(reranked, hasReranker), nil
		},
	}
}
