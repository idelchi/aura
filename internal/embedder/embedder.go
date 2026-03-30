package embedder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"golang.org/x/sync/errgroup"

	chromem "github.com/philippgille/chromem-go"

	"github.com/idelchi/aura/internal/chunker"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/providers"
	"github.com/idelchi/aura/pkg/llm/embedding"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// Embedder manages embedding generation and vector storage.
type Embedder struct {
	provider   providers.Provider
	model      string
	db         *chromem.DB
	collection *chromem.Collection
	chunker    *chunker.Chunker
}

// New creates an Embedder backed by chromem-go persistent storage.
func New(provider providers.Provider, model, dbDir string, chunkCfg chunker.Config) (*Embedder, error) {
	dbPath := folder.New(dbDir).WithFile("index.db").Path()
	if err := folder.New(dbDir).Create(); err != nil {
		return nil, fmt.Errorf("creating db dir: %w", err)
	}

	db, err := chromem.NewPersistentDB(dbPath, true)
	if err != nil {
		return nil, fmt.Errorf("creating chromem db: %w", err)
	}

	collectionName := "index:" + model

	// Clean up stale collections from previous models
	for name := range db.ListCollections() {
		if name != collectionName {
			if err := db.DeleteCollection(name); err != nil {
				debug.Log("[embedder] removing stale collection %q: %v", name, err)
			} else {
				debug.Log("[embedder] removed stale collection %q (model changed)", name)
			}
		}
	}

	collection, err := db.GetOrCreateCollection(collectionName, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("creating collection: %w", err)
	}

	return &Embedder{
		provider:   provider,
		model:      model,
		db:         db,
		collection: collection,
		chunker:    chunker.New(chunkCfg),
	}, nil
}

// embed embeds a single file's chunks. Returns true if embedded, false if skipped (unchanged).
func (e *Embedder) embed(ctx context.Context, path, hash string) (bool, error) {
	// Check if first chunk exists and hash matches (unchanged file)
	firstChunkID := path + "#chunk-0"
	if existing, err := e.collection.GetByID(ctx, firstChunkID); err == nil {
		if storedHash, ok := existing.Metadata["hash"]; ok && storedHash == hash {
			return false, nil
		}
	}

	// Delete existing chunks for this file (content changed)
	if err := e.deleteFileChunks(ctx, path); err != nil {
		return false, err
	}

	data, err := file.New(path).Read()
	if err != nil {
		return false, fmt.Errorf("reading %s: %w", path, err)
	}

	chunks := e.chunker.Chunk(ctx, path, string(data))

	// Batch embed all chunk contents in a single API call
	contents := chunks.Contents()

	emb, ok := providers.As[providers.Embedder](e.provider)
	if !ok {
		return false, errors.New("provider does not support embeddings")
	}

	resp, _, err := emb.Embed(ctx, embedding.Request{
		Model: e.model,
		Input: contents,
	})
	if err != nil {
		return false, fmt.Errorf("embedding %s: %w", path, err)
	}

	if len(resp.Embeddings) != len(chunks) {
		return false, fmt.Errorf(
			"embedding count mismatch for %s: got %d, want %d",
			path,
			len(resp.Embeddings),
			len(chunks),
		)
	}

	// Store each chunk with pre-computed embedding
	for i, chunk := range chunks {
		chunkID := fmt.Sprintf("%s#chunk-%d", path, chunk.Index)

		err := e.collection.AddDocument(ctx, chromem.Document{
			ID:        chunkID,
			Content:   chunk.Content,
			Embedding: toFloat32(resp.Embeddings[i]),
			Metadata: map[string]string{
				"path":        path,
				"hash":        hash,
				"chunk_index": strconv.Itoa(chunk.Index),
				"start_line":  strconv.Itoa(chunk.StartLine),
				"end_line":    strconv.Itoa(chunk.EndLine),
			},
		})
		if err != nil {
			return false, fmt.Errorf("storing chunk %s: %w", chunkID, err)
		}
	}

	return true, nil
}

// deleteFileChunks removes all chunks for a given file path using metadata filtering.
func (e *Embedder) deleteFileChunks(ctx context.Context, path string) error {
	if err := e.collection.Delete(ctx, map[string]string{"path": path}, nil); err != nil {
		return fmt.Errorf("deleting chunks for %s: %w", path, err)
	}

	return nil
}

// Stats tracks embedding operation results.
type Stats struct {
	Embedded int
	Skipped  int
	Removed  int
	Total    int
}

// Display returns a human-readable summary.
func (s Stats) Display() string {
	return fmt.Sprintf("embedded=%d skipped=%d removed=%d total=%d", s.Embedded, s.Skipped, s.Removed, s.Total)
}

// EmbedFiles embeds files in parallel. files is a map of path -> SHA256 hash.
func (e *Embedder) EmbedFiles(ctx context.Context, files map[string]string, concurrency int) (*Stats, error) {
	g := errgroup.Group{}
	g.SetLimit(concurrency)

	var mu sync.Mutex

	stats := &Stats{Total: len(files)}

	for path, hash := range files {
		g.Go(func() error {
			embedded, err := e.embed(ctx, path, hash)
			if err != nil {
				return err
			}

			mu.Lock()

			if embedded {
				stats.Embedded++
			} else {
				stats.Skipped++
			}

			mu.Unlock()

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return stats, err
	}

	return stats, nil
}

// RemoveDeleted removes embeddings for files that were previously indexed
// but are no longer present in the current files map.
// previousPaths is the set of file paths from the previous indexing run.
func (e *Embedder) RemoveDeleted(ctx context.Context, files map[string]string, previousPaths []string) (int, error) {
	removed := 0

	for _, path := range previousPaths {
		if _, exists := files[path]; !exists {
			if err := e.deleteFileChunks(ctx, path); err != nil {
				return removed, fmt.Errorf("removing %s: %w", path, err)
			}

			removed++
		}
	}

	return removed, nil
}

// Query embeds the query string and performs similarity search, returning top-k results.
func (e *Embedder) Query(ctx context.Context, query string, k int) ([]chromem.Result, error) {
	if e.collection.Count() == 0 {
		return nil, nil
	}

	emb, ok := providers.As[providers.Embedder](e.provider)
	if !ok {
		return nil, errors.New("provider does not support embeddings")
	}

	resp, _, err := emb.Embed(ctx, embedding.Request{
		Model: e.model,
		Input: []string{query},
	})
	if err != nil {
		return nil, fmt.Errorf("embedding query: %w", err)
	}

	if len(resp.Embeddings) == 0 {
		return nil, errors.New("no embedding returned for query")
	}

	// Clamp k to collection size
	count := e.collection.Count()
	if k > count {
		k = count
	}

	results, err := e.collection.QueryEmbedding(ctx, toFloat32(resp.Embeddings[0]), k, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("querying: %w", err)
	}

	return results, nil
}

// Load preloads the embedding model on the provider (fire-and-forget).
// Silently no-ops if the provider doesn't support explicit model loading.
func (e *Embedder) Load(ctx context.Context) {
	if loader, ok := providers.As[providers.ModelLoader](e.provider); ok {
		if err := loader.LoadModel(ctx, e.model); err != nil {
			debug.Log("[embedder] preload warning: %v", err)
		}
	}
}

// Unload releases the embedding model from provider memory (fire-and-forget).
// Silently no-ops if the provider doesn't support explicit model unloading.
func (e *Embedder) Unload(ctx context.Context) {
	if loader, ok := providers.As[providers.ModelLoader](e.provider); ok {
		_ = loader.UnloadModel(ctx, e.model)
	}
}

// toFloat32 converts a float64 slice to float32 (providers return float64, chromem-go uses float32).
func toFloat32(f64 []float64) []float32 {
	f32 := make([]float32, len(f64))
	for i, v := range f64 {
		f32[i] = float32(v)
	}

	return f32
}

// HashFile computes the SHA256 hex digest of a file.
func HashFile(path string) (string, error) {
	data, err := file.New(path).Read()
	if err != nil {
		return "", err
	}

	h := sha256.Sum256(data)

	return hex.EncodeToString(h[:]), nil
}
