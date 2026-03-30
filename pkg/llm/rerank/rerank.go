// Package rerank defines the common reranking request and response structures.
package rerank

// Request represents a reranking request.
type Request struct {
	// Model is the reranker model to use.
	Model string
	// Query is the search query to rank documents against.
	Query string
	// Documents is the list of documents to rerank.
	Documents []string
	// TopN limits the number of results returned (0 means return all).
	TopN int
}

// Result represents a single reranked document.
type Result struct {
	// Index is the original position of the document in the input.
	Index int
	// RelevanceScore indicates how relevant the document is to the query.
	RelevanceScore float64
}

// Results is a collection of reranked results.
type Results []Result

// Indices returns the document indices in ranked order.
func (r Results) Indices() []int {
	indices := make([]int, len(r))
	for i, result := range r {
		indices[i] = result.Index
	}

	return indices
}

// Response represents a reranking response.
type Response struct {
	// Model is the model that was used for reranking.
	Model string
	// Results contains the reranked documents with their scores.
	Results Results
}
