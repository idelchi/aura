package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/idelchi/aura/pkg/providers/transport"
	"github.com/idelchi/godyl/pkg/path/file"
)

const authURL = "https://auth.openai.com/oauth/token"

var clientID = "app_EMoamEEZ73f0CkXaXp7hrann"

func init() {
	if id := os.Getenv("AURA_CODEX_CLIENT_ID"); id != "" {
		clientID = id
	}
}

type codexTransport struct {
	mu           sync.RWMutex
	refreshToken string
	authFilePath string
	accessToken  string
	expiry       time.Time
	base         *transport.Transport
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

func newCodexTransport(refreshToken, authFilePath string, timeout time.Duration) *codexTransport {
	return &codexTransport{
		refreshToken: refreshToken,
		authFilePath: authFilePath,
		base:         transport.New(transport.WithTimeout(timeout)),
	}
}

func (t *codexTransport) token(ctx context.Context) (string, error) {
	t.mu.RLock()

	if t.accessToken != "" && time.Now().Before(t.expiry) {
		tok := t.accessToken
		t.mu.RUnlock()

		return tok, nil
	}

	t.mu.RUnlock()

	return t.refresh(ctx)
}

func (t *codexTransport) refresh(ctx context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Re-check after acquiring write lock.
	if t.accessToken != "" && time.Now().Before(t.expiry) {
		return t.accessToken, nil
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", t.refreshToken)
	form.Set("client_id", clientID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, authURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("creating token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	httpClient := &http.Client{Transport: t.base}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("exchanging refresh token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token exchange failed with status %d", resp.StatusCode)
	}

	var result tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}

	// Persist refresh token to disk BEFORE updating in-memory state.
	// If disk write fails, in-memory state stays consistent with what's on disk.
	if result.RefreshToken != "" {
		if t.authFilePath != "" {
			if err := file.New(t.authFilePath).Write([]byte(result.RefreshToken + "\n")); err != nil {
				return "", fmt.Errorf("writing rotated refresh token: %w", err)
			}
		}

		t.refreshToken = result.RefreshToken
	}

	t.accessToken = result.AccessToken
	t.expiry = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)

	return t.accessToken, nil
}

func (t *codexTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	tok, err := t.token(r.Context())
	if err != nil {
		return nil, err
	}

	req := r.Clone(r.Context())
	req.Header.Set("Authorization", "Bearer "+tok)

	return t.base.Base.RoundTrip(req)
}
