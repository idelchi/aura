package commands

import (
	"context"
	"errors"

	"github.com/idelchi/aura/internal/condition"
	"github.com/idelchi/aura/internal/injector"
	"github.com/idelchi/aura/internal/slash"
)

// Assert creates the /assert command for conditional action execution.
func Assert() slash.Command {
	return slash.Command{
		Name:        "/assert",
		Description: "Execute quoted actions if condition is met: /assert [not] <condition> \"action1\" [\"action2\" ...]",
		Hints:       `[not] <condition> "action1" ["action2" ...]`,
		Category:    "execution",
		Silent:      true,
		Execute: func(ctx context.Context, c slash.Context, args ...string) (string, error) {
			if len(args) < 2 {
				return "", errors.New("usage: /assert [not] <condition> \"action1\" [\"action2\" ...]")
			}

			// Parse [not] <condition> <action1> [action2] ...
			i := 0
			negate := false

			if args[i] == "not" {
				negate = true
				i++
			}

			if i >= len(args) {
				return "", errors.New("usage: /assert [not] <condition> \"action1\" [\"action2\" ...]")
			}

			cond := args[i]
			i++

			actions := args[i:]
			if len(actions) == 0 {
				return "", errors.New("usage: /assert [not] <condition> \"action1\" [\"action2\" ...]")
			}

			met := checkCondition(cond, c)

			if negate {
				met = !met
			}

			if !met {
				return "", nil
			}

			for _, action := range actions {
				if err := c.ProcessInput(ctx, action); err != nil {
					return "", err
				}
			}

			return "", nil
		},
	}
}

// checkCondition evaluates an assertion condition using the shared condition package.
// Builds an injector.State and delegates to ConditionState() — single source of truth
// for condition.State construction (shared with the injector system).
func checkCondition(cond string, c slash.Context) bool {
	st := injector.State{
		Iteration: 0, // not tracked in slash context
		Todo: injector.TodoState{
			Pending:    len(c.TodoList().FindPending()),
			InProgress: len(c.TodoList().FindInProgress()),
			Total:      c.TodoList().Len(),
		},
		Auto: c.Resolved().Auto,
		Tokens: injector.TokenSnapshot{
			Estimate: c.Status().Tokens.Used,
			Percent:  c.Status().Tokens.Percent,
			Max:      c.Status().Tokens.Max,
		},
		MessageCount: c.Builder().Len(),
		Stats:        c.SessionStats().Snapshot(),
		Model:        c.ResolvedModel(),
	}

	return condition.Check(cond, st.ConditionState())
}
