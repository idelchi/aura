package llamacpp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/providers/catalog"
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
			catalog.Model
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
		m := x.Model.Model()
		// Router launch arguments describe the configured window, rather than training limits.
		if n := parseCtxSize(x.Status.Args); n > 0 {
			m.ContextLength = model.ContextLength(n)
		}
		models = append(models, m)
	}

	return models, nil
}
