package llamacpp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/providers"
)

// Estimate tokenizes the content and returns the exact token count.
func (c *Client) Estimate(
	ctx context.Context,
	req request.Request,
	content string,
) (int, error) {
	endpoint, err := c.WithEndpoint("/tokenize")
	if err != nil {
		return 0, err
	}

	payload := struct {
		Model        string `json:"model"`
		Content      string `json:"content"`
		ParseSpecial bool   `json:"parse_special"`
	}{
		Model:        req.Model.Name,
		Content:      content,
		ParseSpecial: false,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint.String(),
		bytes.NewReader(body),
	)
	if err != nil {
		return 0, err
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, providers.ClassifyHTTPError(resp.StatusCode, "llamacpp", resp.Status, 0)
	}

	var out struct {
		Tokens []int `json:"tokens"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}

	return len(out.Tokens), nil
}
