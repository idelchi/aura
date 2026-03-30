package query

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/MakeNowJust/heredoc/v2"
	cli "github.com/urfave/cli/v3"

	"github.com/idelchi/aura/internal/agent"
	"github.com/idelchi/aura/internal/cli/core"
	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/indexer"
	"github.com/idelchi/aura/pkg/spinner"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// Command creates the 'aura query' subcommand for embedding-based codebase search.
func Command(flags *core.Flags) *cli.Command {
	return &cli.Command{
		Name:  "query",
		Usage: "Embedding-based search across the codebase",
		Before: func(_ context.Context, _ *cli.Command) (context.Context, error) {
			flags := core.GetFlags()
			debug.Init(flags.WriteHome(), flags.Debug)

			return nil, nil
		},
		After: func(_ context.Context, _ *cli.Command) error {
			debug.Close()

			return nil
		},
		Description: heredoc.Doc(`
			Query performs embedding-based search across the codebase.

			The codebase is indexed (or incrementally updated) on each invocation,
			then searched for the most relevant chunks matching the query terms.
			Results show file paths, line ranges, and similarity scores.

			When a reranker agent is configured, both unsorted and reranked
			results are displayed.
		`) + "\n\nExamples:\n" + heredoc.Doc(`
			# Search for token counting logic
			aura query "token counting"

			# Search with more results
			aura query -k 10 "configuration loading"

			# Reindex only (no search)
			aura query
		`),
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:        "top",
				Aliases:     []string{"k"},
				Usage:       "Number of top results (default: from config)",
				Value:       0,
				Destination: &flags.Query.Top,
				Sources:     cli.EnvVars("AURA_QUERY_TOP"),
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := core.GetFlags()

			args := cmd.Args().Slice()

			cfg, paths, err := config.New(flags.ConfigOptions(),
				config.PartFeatures, config.PartAgents, config.PartProviders,
				config.PartModes, config.PartSystems, config.PartAgentsMd,
			)
			if err != nil {
				return err
			}

			searchCfg := cfg.Features.Embeddings

			if searchCfg.Agent == "" {
				return errors.New("embeddings requires an agent configured in features/embeddings.yaml")
			}

			searchAgent, err := agent.New(cfg, paths, nil, searchCfg.Agent)
			if err != nil {
				return fmt.Errorf("creating search agent: %w", err)
			}

			estimator, err := cfg.Features.Estimation.NewEstimator()
			if err != nil {
				return fmt.Errorf("creating estimator: %w", err)
			}

			chunkCfg := searchCfg.Chunking.ChunkerConfig(estimator.EstimateLocal)

			gi := indexer.BuildGitignore(searchCfg.Gitignore)

			idx, err := indexer.New(
				searchAgent.Provider,
				searchAgent.Model.Name,
				folder.New(paths.Home, "embeddings").Path(),
				chunkCfg,
				gi,
			)
			if err != nil {
				return fmt.Errorf("creating indexer: %w", err)
			}

			hasReranker := searchCfg.Reranking.Agent != ""
			if hasReranker {
				rerankAgent, err := agent.New(cfg, paths, nil, searchCfg.Reranking.Agent)
				if err != nil {
					return fmt.Errorf("creating rerank agent: %w", err)
				}

				idx.UseReranker(rerankAgent.Provider, rerankAgent.Model.Name)
			}

			idx.SetOffload(searchCfg.Offload, searchCfg.Reranking.Offload)

			s := spinner.New("Preparing...")

			s.Start()
			defer s.Stop()

			idx.OnProgress = func(phase string) {
				s.Update(phase)
			}

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()

			stats, err := idx.Index(ctx)
			if err != nil {
				s.Fail("Indexing failed")

				return fmt.Errorf("indexing: %w", err)
			}

			if len(args) == 0 {
				s.Success("Index: " + stats.Display())

				return nil
			}

			k := flags.Query.Top
			if k <= 0 {
				k = searchCfg.MaxResults
			}

			query := strings.Join(args, " ")

			unsorted, reranked, err := idx.Query(ctx, query, k, searchCfg.Reranking.Multiplier)
			if err != nil {
				s.Fail("Query failed")

				return fmt.Errorf("querying: %w", err)
			}

			s.Success("Index: " + stats.Display())

			fmt.Fprintln(cmd.Writer, unsorted.DisplayWithReranked(reranked, hasReranker))

			return nil
		},
	}
}
