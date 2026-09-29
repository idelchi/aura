package llamacpp

import (
	"context"
	"fmt"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/providers/capabilities"
)

// Model fetches metadata for the specified model.
func (c *Client) Model(ctx context.Context, name string) (model.Model, error) {
	info, err := c.Show(ctx, name)
	if err != nil {
		return model.Model{}, fmt.Errorf("fetching model info for %q: %w", name, err)
	}

	m := model.Model{
		Name:           name,
		ParameterCount: model.ParseParameterName(name),
		ContextLength:  model.ContextLength(info.DefaultGenerationSettings.ContextLength),
	}

	m = WithCapabilities(m, info)
	if info.ChatTemplate != "" && !m.Knows(capabilities.Thinking) {
		supported, err := c.supportsThinking(ctx, name)
		if err != nil {
			debug.Log("[llamacpp] thinking capability unknown for %s: %v", name, err)
		} else if supported {
			m.SetCapability(capabilities.Thinking, true)
		}
	}
	return m, nil
}
