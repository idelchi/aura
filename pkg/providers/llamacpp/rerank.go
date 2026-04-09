package llamacpp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/idelchi/aura/pkg/llm/rerank"
)

// rerankRequest is the API request format for llama.cpp reranking.
type rerankRequest struct {
	Model     string   `json:"model,omitempty"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n,omitempty"`
}

// rerankResult is a single result from the rerank API.
type rerankResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

// rerankResponse is the API response format for llama.cpp reranking.
type rerankResponse struct {
	Results []rerankResult `json:"results"`
	Model   string         `json:"model"`
}

// Rerank sends documents to be reranked against a query.
func (c *Client) Rerank(ctx context.Context, req rerank.Request) (rerank.Response, error) {
	u, err := c.WithEndpoint("/v1/rerank")
	if err != nil {
		return rerank.Response{}, fmt.Errorf("building URL: %w", err)
	}

	apiReq := rerankRequest{
		Model:     req.Model,
		Query:     req.Query,
		Documents: req.Documents,
		TopN:      req.TopN,
	}

	body, err := json.Marshal(apiReq)
	if err != nil {
		return rerank.Response{}, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return rerank.Response{}, fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return rerank.Response{}, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var apiErr Error
		if err := apiErr.FromResponse(resp); err != nil {
			return rerank.Response{}, fmt.Errorf("rerank failed with status %d", resp.StatusCode)
		}

		return rerank.Response{}, &apiErr
	}

	var apiResp rerankResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return rerank.Response{}, fmt.Errorf("decoding response: %w", err)
	}

	results := make(rerank.Results, len(apiResp.Results))
	for i, r := range apiResp.Results {
		results[i] = rerank.Result{
			Index:          r.Index,
			RelevanceScore: r.RelevanceScore,
		}
	}

	return rerank.Response{
		Model:   apiResp.Model,
		Results: results,
	}, nil
}
