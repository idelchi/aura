package gotify

import (
	"context"
	"fmt"
	"os"

	"github.com/idelchi/aura/sdk"
)

// Execute sends a push notification via the Gotify REST API.
func Execute(ctx context.Context, _ sdk.Context, args map[string]any) (string, error) {
	baseURL := os.Getenv("GARFIELD_LABS_GOTIFY_URL")
	if baseURL == "" {
		return "", fmt.Errorf("GARFIELD_LABS_GOTIFY_URL is not set — load it via aura --env-file")
	}

	token := os.Getenv("GARFIELD_LABS_GOTIFY_TOKEN")
	if token == "" {
		return "", fmt.Errorf("GARFIELD_LABS_GOTIFY_TOKEN is not set — load it via aura --env-file")
	}

	title, ok := args["title"].(string)
	if !ok {
		return "", fmt.Errorf("title: expected string")
	}

	message, ok := args["message"].(string)
	if !ok {
		return "", fmt.Errorf("message: expected string")
	}

	rawLevel := "INFO"
	if l, ok := args["level"].(string); ok {
		rawLevel = l
	}

	level, err := ParseLevel(rawLevel)
	if err != nil {
		return "", err
	}

	if minRaw := os.Getenv("GOTIFY_LOG_LEVEL"); minRaw != "" {
		minLevel, err := ParseLevel(minRaw)
		if err != nil {
			return "", fmt.Errorf("invalid GOTIFY_LOG_LEVEL: %w", err)
		}
		if !level.ShouldSend(minLevel) {
			return fmt.Sprintf("Notification suppressed [%s]: %s (minimum level: %s)", level, title, minLevel), nil
		}
	}

	if err := Send(ctx, baseURL, token, level, title, message); err != nil {
		return "", err
	}

	return fmt.Sprintf("Notification sent [%s]: %s", level, title), nil
}
