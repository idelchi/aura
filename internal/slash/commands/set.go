package commands

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/idelchi/aura/internal/slash"
	"github.com/idelchi/aura/pkg/llm/thinking"
	"github.com/idelchi/aura/pkg/truthy"
)

// Set creates the /set command for programmatic runtime configuration.
func Set() slash.Command {
	return slash.Command{
		Name:        "/set",
		Description: "Set runtime parameters (key=value pairs)",
		Hints:       "key=value [key=value ...]",
		Category:    "config",
		Execute: func(_ context.Context, c slash.Context, args ...string) (string, error) {
			if len(args) == 0 {
				return "", slash.ErrUsage
			}

			var results []string

			for _, arg := range args {
				key, value, ok := strings.Cut(arg, "=")
				if !ok {
					return "", fmt.Errorf("invalid format %q (expected key=value): %w", arg, slash.ErrUsage)
				}

				msg, err := applySetPair(c, key, value)
				if err != nil {
					return "", err
				}

				results = append(results, msg)
			}

			return strings.Join(results, "\n"), nil
		},
	}
}

func applySetPair(c slash.Context, key, value string) (string, error) {
	switch key {
	case "thinking", "think":
		think, err := thinking.ParseValue(value)
		if err != nil {
			return "", err
		}

		if err := c.SetThink(think); err != nil {
			return "", err
		}

		return fmt.Sprintf("thinking=%s", think), nil

	case "mode":
		if err := c.SwitchMode(value); err != nil {
			return "", err
		}

		return "mode=" + value, nil

	case "agent":
		if err := c.SwitchAgent(value, "user"); err != nil {
			return "", err
		}

		return "agent=" + value, nil

	case "auto":
		b, err := truthy.Parse(value)
		if err != nil {
			return "", fmt.Errorf("invalid value for auto: %w", err)
		}

		c.SetAuto(b)

		return fmt.Sprintf("auto=%v", b), nil

	case "sandbox":
		b, err := truthy.Parse(value)
		if err != nil {
			return "", fmt.Errorf("invalid value for sandbox: %w", err)
		}

		if err := c.SetSandbox(b); err != nil {
			return "", err
		}

		return fmt.Sprintf("sandbox=%v", b), nil

	case "context", "window":
		size, err := strconv.Atoi(value)
		if err != nil {
			return "", fmt.Errorf("invalid int for context: %q", value)
		}

		if err := c.ResizeContext(size); err != nil {
			return "", err
		}

		return fmt.Sprintf("context=%d", size), nil

	case "readbefore.write", "rb.write":
		val, err := truthy.Parse(value)
		if err != nil {
			return "", fmt.Errorf("invalid value for readbefore.write: %w", err)
		}

		p := c.ReadBeforePolicy()

		p.Write = val

		if err := c.SetReadBeforePolicy(p); err != nil {
			return "", fmt.Errorf("applying read-before policy: %w", err)
		}

		return fmt.Sprintf("readbefore.write=%v", val), nil

	case "readbefore.delete", "rb.delete":
		val, err := truthy.Parse(value)
		if err != nil {
			return "", fmt.Errorf("invalid value for readbefore.delete: %w", err)
		}

		p := c.ReadBeforePolicy()

		p.Delete = val

		if err := c.SetReadBeforePolicy(p); err != nil {
			return "", fmt.Errorf("applying read-before policy: %w", err)
		}

		return fmt.Sprintf("readbefore.delete=%v", val), nil

	case "verbose":
		b, err := truthy.Parse(value)
		if err != nil {
			return "", fmt.Errorf("invalid value for verbose: %w", err)
		}

		c.SetVerbose(b)

		return fmt.Sprintf("verbose=%v", b), nil

	case "done":
		b, err := truthy.Parse(value)
		if err != nil {
			return "", fmt.Errorf("invalid value for done: %w", err)
		}

		if err := c.SetDone(b); err != nil {
			return "", err
		}

		return fmt.Sprintf("done=%v", b), nil

	default:
		return "", fmt.Errorf("unknown key %q", key)
	}
}
