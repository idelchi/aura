package llamacpp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/idelchi/aura/pkg/llm/model"
)

// parseCtxSize extracts the --ctx-size value from a LlamaCPP status.args slice.
// Returns 0 if --ctx-size is absent or unparseable.
func parseCtxSize(args []string) int {
	for i, arg := range args {
		if arg == "--ctx-size" && i+1 < len(args) {
			if v, err := strconv.Atoi(args[i+1]); err == nil {
				return v
			}
		}
	}

	return 0
}

// List fetches the list of available models from the /models endpoint.
func (c *Client) List(ctx context.Context) (model.Models, error) {
	endpoint, err := c.WithEndpoint("/models")
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var apiErr Error
		if err := apiErr.FromResponse(resp); err != nil {
			return nil, fmt.Errorf("listing models: %s", resp.Status)
		}

		return nil, &apiErr
	}

	response := struct {
		Data []struct {
			ID     string `json:"id"`
			Status struct {
				Args []string `json:"args"`
			} `json:"status"`
		} `json:"data"`
	}{}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	var models model.Models

	for _, x := range response.Data {
		// ContextLength from --ctx-size in status.args — the configured (not native) context window.
		// Only present when the server explicitly sets it; 0 means unconfigured (same as before).
		// ParameterCount parsed from name — API has no field for it.
		models = append(models, model.Model{
			Name:           x.ID,
			ParameterCount: model.ParseParameterName(x.ID),
			ContextLength:  model.ContextLength(parseCtxSize(x.Status.Args)),
		})
	}

	return models, nil
}
