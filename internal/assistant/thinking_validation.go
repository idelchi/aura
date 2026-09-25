package assistant

import (
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/thinking"
)

func (a *Assistant) validateThinkForResolvedModel(value thinking.Value) (thinking.Value, error) {
	if a.noopProvider != nil {
		return value, nil
	}

	if a.resolved.model == nil {
		return value, nil
	}

	return a.resolved.model.NormalizeThinking(value, false)
}

func (a *Assistant) normalizeCurrentThinkForModel(m model.Model, coerce bool) error {
	if a.noopProvider != nil {
		return nil
	}

	normalized, err := m.NormalizeThinking(a.agent.Model.Think, coerce)
	if err != nil {
		return err
	}

	a.agent.Model.Think = normalized

	return nil
}
