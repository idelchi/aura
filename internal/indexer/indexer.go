package indexer

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/charlievieth/fastwalk"
	chromem "github.com/philippgille/chromem-go"

	"github.com/idelchi/aura/internal/chunker"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/embedder"
	"github.com/idelchi/aura/internal/providers"
	"github.com/idelchi/aura/pkg/llm/rerank"
	"github.com/idelchi/go-gitignore"
	"github.com/idelchi/godyl/pkg/path/file"
)

// Result represents a single search result with file location and similarity score.
type Result struct {
	Path       string  `json:"path"`
	Content    string  `json:"content,omitempty"`
	StartLine  int     `json:"start_line"`
	EndLine    int     `json:"end_line"`
	Similarity float32 `json:"similarity"`
}

// Display returns a human-readable summary of a single result.
func (r Result) Display() string {
	return fmt.Sprintf("%s:%d-%d (%.2f)", r.Path, r.StartLine, r.EndLine, r.Similarity)
}

// Results is a collection of search results.
type Results []Result

// Display returns a human-readable summary of all results.
func (rs Results) Display() string {
	if len(rs) == 0 {
		return "No results found"
	}

	var b strings.Builder

	for i, r := range rs {
		if i > 0 {
			b.WriteString("\n")
		}

		b.WriteString(r.Display())
	}

	return b.String()
}

// DisplayWithReranked formats both unsorted and reranked results for display.
// When hasReranker is true, shows both sections; otherwise shows just the results.
func (rs Results) DisplayWithReranked(reranked Results, hasReranker bool) string {
	if !hasReranker {
		return rs.Display()
	}

	var b strings.Builder

	b.WriteString("Unsorted:\n")
	b.WriteString(rs.Display())
	b.WriteString("\n\nReranked:\n")
	b.WriteString(reranked.Display())

	return b.String()
}

// Indexer manages file walking, embedding, querying, and reranking.
type Indexer struct {
	embedder       *embedder.Embedder
	rerankProvider providers.Provider
	rerankModel    string
	gi             *gitignore.GitIgnore
	files          map[string]string
	previousPaths  []string
	offloadEmbed   bool
	offloadRerank  bool

	// OnProgress is called at each phase transition with a human-readable status message.
	// If nil, progress is silently ignored.
	OnProgress func(phase string)
}

// Progress calls OnProgress if set.
func (idx *Indexer) Progress(phase string) {
	if idx.OnProgress != nil {
		idx.OnProgress(phase)
	}
}

// New creates an Indexer with the given provider, model, storage directory, and gitignore.
func New(
	provider providers.Provider,
	embedModel, dbDir string,
	chunkCfg chunker.Config,
	gi *gitignore.GitIgnore,
) (*Indexer, error) {
	emb, err := embedder.New(provider, embedModel, dbDir, chunkCfg)
	if err != nil {
		return nil, fmt.Errorf("creating embedder: %w", err)
	}

	return &Indexer{
		embedder: emb,
		gi:       gi,
		files:    make(map[string]string),
	}, nil
}

// UseReranker sets the provider and model to use for reranking.
func (idx *Indexer) UseReranker(provider providers.Provider, model string) {
	idx.rerankProvider = provider
	idx.rerankModel = model
}

// SetOffload enables explicit model load/unload for embedding and/or reranking.
// When enabled, models are loaded before use and unloaded after to free VRAM.
func (idx *Indexer) SetOffload(embed, rerank bool) {
	idx.offloadEmbed = embed
	idx.offloadRerank = rerank
}

// Walk scans the directory tree, respecting gitignore rules, and computes file hashes.
func (idx *Indexer) Walk(dir string) error {
	files := make(map[string]string)

	var mu sync.Mutex

	conf := &fastwalk.Config{NumWorkers: 20}

	err := fastwalk.Walk(conf, dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		path = strings.TrimPrefix(filepath.ToSlash(path), "./")

		if idx.gi != nil && idx.gi.Ignored(path, false) {
			return nil
		}

		hash, err := embedder.HashFile(path)
		if err != nil {
			debug.Log("[indexer] hash failed for %s: %v", path, err)

			return nil
		}

		mu.Lock()

		files[path] = hash

		mu.Unlock()

		return nil
	})
	if err != nil {
		return fmt.Errorf("walking %s: %w", dir, err)
	}

	idx.previousPaths = slices.Collect(maps.Keys(idx.files))
	idx.files = files

	return nil
}

// Index walks the directory and embeds files, returning stats.
func (idx *Indexer) Index(ctx context.Context) (*embedder.Stats, error) {
	if idx.offloadEmbed {
		idx.embedder.Load(ctx)
	}

	idx.Progress("Scanning files...")

	if err := idx.Walk("."); err != nil {
		return nil, fmt.Errorf("walking files: %w", err)
	}

	if len(idx.files) == 0 {
		return &embedder.Stats{}, nil
	}

	idx.Progress("Embedding files...")

	stats, err := idx.embedder.EmbedFiles(ctx, idx.files, 20)
	if err != nil {
		return stats, fmt.Errorf("embedding files: %w", err)
	}

	idx.Progress("Cleaning up...")

	removed, err := idx.embedder.RemoveDeleted(ctx, idx.files, idx.previousPaths)
	if err != nil {
		return stats, fmt.Errorf("removing deleted: %w", err)
	}

	stats.Removed = removed

	return stats, nil
}

// Query searches the index and returns both unsorted and reranked results.
// When no reranker is set, both return values are identical.
func (idx *Indexer) Query(ctx context.Context, query string, k, rerankMultiplier int) (Results, Results, error) {
	idx.Progress("Searching...")

	// Fetch more candidates than needed for dedup and reranking
	fetchK := max(k*rerankMultiplier, 10)

	raw, err := idx.embedder.Query(ctx, query, fetchK)
	if err != nil {
		return nil, nil, fmt.Errorf("querying embeddings: %w", err)
	}

	if idx.offloadEmbed {
		idx.embedder.Unload(ctx)
	}

	if len(raw) == 0 {
		return nil, nil, nil
	}

	results := toResults(raw)

	unsorted := dedup(results, k)

	// If no reranker, return identical results
	if idx.rerankProvider == nil {
		return unsorted, unsorted, nil
	}

	if idx.offloadRerank {
		if loader, ok := providers.As[providers.ModelLoader](idx.rerankProvider); ok {
			_ = loader.LoadModel(ctx, idx.rerankModel)

			defer func() { _ = loader.UnloadModel(ctx, idx.rerankModel) }()
		}
	}

	idx.Progress("Reranking...")

	reranked, err := idx.Rerank(ctx, query, results)
	if err != nil {
		return nil, nil, fmt.Errorf("reranking: %w", err)
	}

	return unsorted, dedup(reranked, k), nil
}

// Rerank reorders results using the rerank provider.
func (idx *Indexer) Rerank(ctx context.Context, query string, results Results) (Results, error) {
	docs := make([]string, len(results))
	for i, r := range results {
		docs[i] = r.Content
	}

	reranker, ok := providers.As[providers.Reranker](idx.rerankProvider)
	if !ok {
		return nil, errors.New("provider does not support reranking")
	}

	resp, err := reranker.Rerank(ctx, rerank.Request{
		Model:     idx.rerankModel,
		Query:     query,
		Documents: docs,
	})
	if err != nil {
		return nil, err
	}

	reordered := make(Results, 0, len(resp.Results))
	for _, rr := range resp.Results {
		if rr.Index < len(results) {
			r := results[rr.Index]

			r.Similarity = float32(rr.RelevanceScore)
			reordered = append(reordered, r)
		}
	}

	return reordered, nil
}

// toResults converts chromem results to indexer results.
func toResults(raw []chromem.Result) Results {
	results := make(Results, 0, len(raw))
	for _, r := range raw {
		startLine, _ := strconv.Atoi(r.Metadata["start_line"])
		endLine, _ := strconv.Atoi(r.Metadata["end_line"])
		path := r.Metadata["path"]

		if path == "" {
			if idx := strings.Index(r.ID, "#chunk-"); idx >= 0 {
				path = r.ID[:idx]
			} else {
				path = r.ID
			}
		}

		results = append(results, Result{
			Path:       path,
			Content:    r.Content,
			StartLine:  startLine,
			EndLine:    endLine,
			Similarity: r.Similarity,
		})
	}

	return results
}

// dedup removes duplicate file entries, keeping the highest-scored occurrence per file.
func dedup(results Results, limit int) Results {
	seen := make(map[string]bool)
	deduped := make(Results, 0, limit)

	for _, r := range results {
		if seen[r.Path] {
			continue
		}

		seen[r.Path] = true
		deduped = append(deduped, r)

		if len(deduped) >= limit {
			break
		}
	}

	return deduped
}

// BuildGitignore combines .gitignore from the filesystem with config-defined patterns.
func BuildGitignore(configPatterns string) *gitignore.GitIgnore {
	gi := gitignore.New()

	// Load filesystem .gitignore if it exists
	if data, err := file.New(".gitignore").Read(); err == nil {
		lines := strings.Split(string(data), "\n")
		gi.Append(lines...)
	}

	// Append config-defined patterns
	if configPatterns != "" {
		lines := strings.Split(configPatterns, "\n")
		gi.Append(lines...)
	}

	return gi
}
