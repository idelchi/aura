// Package gotify provides HTTP transport for sending Gotify push notifications.
package gotify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Send posts a notification to the Gotify REST API.
func Send(ctx context.Context, baseURL, token string, level Level, title, message string) error {
	payload, err := json.Marshal(map[string]any{
		"title":    fmt.Sprintf("[%s]: %s", level, title),
		"message":  message,
		"priority": level.Priority(),
	})
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}

	endpoint := strings.TrimRight(baseURL, "/") + "/message?token=" + token

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("sending notification: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gotify returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
