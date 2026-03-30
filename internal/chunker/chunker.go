package chunker

import (
	"context"
	"strings"
)

// Chunk represents a piece of a file with positional metadata.
type Chunk struct {
	Index     int    // 0-based chunk index within the file.
	Content   string // Chunk text.
	StartLine int    // 1-based start line (inclusive).
	EndLine   int    // 1-based end line (inclusive).
}

// Chunks is a collection of chunks from a single file.
type Chunks []Chunk

// Contents returns the text content of all chunks.
func (cs Chunks) Contents() []string {
	contents := make([]string, len(cs))
	for i, c := range cs {
		contents[i] = c.Content
	}

	return contents
}

// Strategy determines which chunking algorithm to use.
type Strategy string

const (
	StrategyAuto Strategy = "auto" // Choose AST if supported, otherwise line-based.
	StrategyAST  Strategy = "ast"  // Parse AST and chunk by syntax nodes.
	StrategyLine Strategy = "line" // Line-based chunking only.
)

// Config holds chunking parameters.
type Config struct {
	Strategy      Strategy         // Chunking strategy (default: auto).
	MaxTokens     int              // Target max tokens per chunk (default: 500).
	OverlapTokens int              // Overlap tokens between adjacent chunks (default: 75).
	Estimate      func(string) int // Token estimation function.
}

// ApplyDefaults sets defaults for zero-valued fields.
func (c *Config) ApplyDefaults() {
	if c.Strategy == "" {
		c.Strategy = StrategyAuto
	}

	if c.MaxTokens == 0 {
		c.MaxTokens = 500
	}

	if c.OverlapTokens == 0 {
		c.OverlapTokens = 75
	}
}

// Chunker splits file content into overlapping chunks.
type Chunker struct {
	config Config
	ast    *astChunker
}

// Estimate returns the token count for text using the configured estimator.
func (c *Chunker) Estimate(text string) int {
	return c.config.Estimate(text)
}

// New creates a Chunker with the given config.
func New(config Config) *Chunker {
	config.ApplyDefaults()

	c := &Chunker{config: config}

	c.ast = newastChunker(config.MaxTokens, config.OverlapTokens, c)

	return c
}

// Chunk splits content into token-limited chunks with overlap.
// Small files that fit within MaxTokens are returned as a single chunk.
func (c *Chunker) Chunk(ctx context.Context, path, content string) Chunks {
	if c.Estimate(content) <= c.config.MaxTokens {
		return Chunks{{
			Index:     0,
			Content:   content,
			StartLine: 1,
			EndLine:   countLines(content),
		}}
	}

	strategy := c.config.Strategy
	if strategy == StrategyAuto {
		if c.ast.SupportsExtension(path) {
			strategy = StrategyAST
		} else {
			strategy = StrategyLine
		}
	}

	switch strategy {
	case StrategyAST:
		chunks, err := c.ast.Chunk(ctx, path, content)
		if err != nil {
			return c.chunkByLines(content)
		}

		return chunks
	default:
		return c.chunkByLines(content)
	}
}

// chunkByLines splits content into line-based chunks with token limits and overlap.
func (c *Chunker) chunkByLines(content string) Chunks {
	lines := strings.Split(content, "\n")

	var chunks Chunks

	var currentLines []string

	startLine := 1
	currentTokens := 0

	for i, line := range lines {
		lineNum := i + 1
		lineTokens := c.Estimate(line)

		if currentTokens+lineTokens > c.config.MaxTokens && len(currentLines) > 0 {
			chunks = append(chunks, Chunk{
				Index:     len(chunks),
				Content:   strings.Join(currentLines, "\n"),
				StartLine: startLine,
				EndLine:   lineNum - 1,
			})

			// Compute overlap from the end of the current chunk
			overlapStart := c.OverlapStart(lines, lineNum-1)

			currentLines = nil
			currentTokens = 0

			for j := overlapStart - 1; j < lineNum-1; j++ {
				currentLines = append(currentLines, lines[j])

				currentTokens += c.Estimate(lines[j])
			}

			startLine = overlapStart
		}

		currentLines = append(currentLines, line)

		currentTokens += lineTokens
	}

	// Flush remaining lines
	if len(currentLines) > 0 {
		chunks = append(chunks, Chunk{
			Index:     len(chunks),
			Content:   strings.Join(currentLines, "\n"),
			StartLine: startLine,
			EndLine:   len(lines),
		})
	}

	return chunks
}

// OverlapStart walks backward from endLine to find where overlap should begin.
// Returns a 1-based line number.
func (c *Chunker) OverlapStart(lines []string, endLine int) int {
	toks := 0

	for i := endLine - 1; i >= 0; i-- {
		toks += c.Estimate(lines[i])
		if toks >= c.config.OverlapTokens {
			return i + 1 // 1-based
		}
	}

	return 1
}

// countLines counts the number of lines in a string.
func countLines(s string) int {
	if s == "" {
		return 0
	}

	return strings.Count(s, "\n") + 1
}
