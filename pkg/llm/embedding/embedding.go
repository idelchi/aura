// Package embedding defines common types for embedding operations.
package embedding

// Request represents an embedding request.
type Request struct {
	// Model is the model name to use for embeddings.
	Model string
	// Input is the list of text inputs to embed.
	Input []string
}

// Response represents an embedding response.
type Response struct {
	// Model is the model used for the embeddings.
	Model string
	// Embeddings contains the embedding vectors (one per input).
	Embeddings [][]float64
}
