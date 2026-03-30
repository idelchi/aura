package config

import "github.com/idelchi/aura/internal/chunker"

// Embeddings holds configuration for embedding-based search and reranking.
type Embeddings struct {
	// Agent is the name of the agent to use for embedding.
	Agent string `yaml:"agent"`
	// MaxResults is the default number of results to return.
	MaxResults int `yaml:"max_results"`
	// Gitignore is additional gitignore patterns for file filtering.
	// Combined with .gitignore at project root if it exists.
	Gitignore string `yaml:"gitignore"`
	// Offload enables explicit model load/unload for the embedding model.
	// When true, the embed model is preloaded before indexing and unloaded after query embedding
	// to free VRAM for subsequent pipeline stages (e.g., reranking).
	Offload bool `yaml:"offload"`
	// Chunking holds chunking parameters.
	Chunking EmbeddingChunking `yaml:"chunking"`
	// Reranking holds reranking configuration.
	Reranking EmbeddingReranking `yaml:"reranking"`
}

// EmbeddingChunking holds chunking parameters for embedding-based search.
type EmbeddingChunking struct {
	// Strategy is the chunking strategy to use.
	Strategy string `validate:"omitempty,oneof=auto ast line" yaml:"strategy"`
	// MaxTokens is the target max tokens per chunk.
	MaxTokens int `yaml:"max_tokens"`
	// OverlapTokens is the overlap tokens between adjacent chunks.
	OverlapTokens int `yaml:"overlap_tokens"`
}

// ChunkerConfig converts the YAML-sourced chunking settings into the typed chunker.Config.
func (c EmbeddingChunking) ChunkerConfig(estimate func(string) int) chunker.Config {
	return chunker.Config{
		Strategy:      chunker.Strategy(c.Strategy),
		MaxTokens:     c.MaxTokens,
		OverlapTokens: c.OverlapTokens,
		Estimate:      estimate,
	}
}

// EmbeddingReranking holds reranking configuration for embedding-based search.
type EmbeddingReranking struct {
	// Agent is the name of the agent to use for reranking (empty = skip reranking).
	Agent string `yaml:"agent"`
	// Multiplier controls how many extra candidates to fetch for reranking.
	Multiplier int `yaml:"multiplier"`
	// Offload enables explicit model load/unload for the reranker model.
	// When true, the reranker model is loaded before reranking and unloaded after
	// to free VRAM.
	Offload bool `yaml:"offload"`
}

// ApplyDefaults sets sane defaults for zero-valued fields.
func (s *Embeddings) ApplyDefaults() error {
	if s.MaxResults == 0 {
		s.MaxResults = 5
	}

	if s.Chunking.Strategy == "" {
		s.Chunking.Strategy = "auto"
	}

	if s.Chunking.MaxTokens == 0 {
		s.Chunking.MaxTokens = 500
	}

	if s.Chunking.OverlapTokens == 0 {
		s.Chunking.OverlapTokens = 75
	}

	if s.Reranking.Multiplier == 0 {
		s.Reranking.Multiplier = 4
	}

	return nil
}
