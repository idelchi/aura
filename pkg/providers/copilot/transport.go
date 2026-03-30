package copilot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/idelchi/aura/pkg/providers/transport"
)

const githubTokenURL = "https://api.github.com/copilot_internal/v2/token"

const (
	copilotUserAgent     = "GitHubCopilotChat/0.32.4"
	copilotEditorVersion = "vscode/1.105.1"
	copilotPluginVersion = "copilot-chat/0.32.4"
)

type copilotToken struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
	Endpoints struct {
		API string `json:"api"`
	} `json:"endpoints"`
}

type copilotTransport struct {
	ghToken   string
	base      *transport.Transport
	mu        sync.RWMutex
	token     string
	expiresAt int64
	baseURL   string
}

func newTransport(ghToken string, timeout time.Duration) *copilotTransport {
	return &copilotTransport{
		ghToken: ghToken,
		base:    transport.New(transport.WithTimeout(timeout)),
	}
}

func (t *copilotTransport) ensureToken(ctx context.Context) error {
	t.mu.RLock()

	now := time.Now().Unix()
	valid := t.token != "" && t.expiresAt > now+60
	t.mu.RUnlock()

	if valid {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Re-check after acquiring write lock.
	now = time.Now().Unix()
	if t.token != "" && t.expiresAt > now+60 {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubTokenURL, nil)
	if err != nil {
		return fmt.Errorf("building token request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+t.ghToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", copilotUserAgent)
	req.Header.Set("Editor-Version", copilotEditorVersion)
	req.Header.Set("Editor-Plugin-Version", copilotPluginVersion)

	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return fmt.Errorf("fetching copilot token: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("copilot token exchange returned status %d", resp.StatusCode)
	}

	var ct copilotToken
	if err := json.NewDecoder(resp.Body).Decode(&ct); err != nil {
		return fmt.Errorf("decoding copilot token response: %w", err)
	}

	if ct.Token == "" {
		return errors.New("copilot token response contained empty token")
	}

	t.token = ct.Token
	t.expiresAt = ct.ExpiresAt
	t.baseURL = ct.Endpoints.API

	return nil
}

// BaseURL returns the current Copilot API base URL or a placeholder if not yet exchanged.
func (t *copilotTransport) BaseURL() string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.baseURL != "" {
		return t.baseURL
	}

	return "https://copilot.placeholder"
}

func (t *copilotTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.ensureToken(req.Context()); err != nil {
		return nil, err
	}

	t.mu.RLock()

	token := t.token
	baseURL := t.baseURL
	t.mu.RUnlock()

	cloned := req.Clone(req.Context())

	// Strip SDK-injected auth headers; we set our own.
	cloned.Header.Del("X-Api-Key")
	cloned.Header.Del("Authorization")

	cloned.Header.Set("Authorization", "Bearer "+token)
	cloned.Header.Set("User-Agent", copilotUserAgent)
	cloned.Header.Set("Editor-Version", copilotEditorVersion)
	cloned.Header.Set("Editor-Plugin-Version", copilotPluginVersion)
	cloned.Header.Set("Copilot-Integration-Id", "vscode-chat")
	cloned.Header.Set("Anthropic-Version", "2023-06-01")
	cloned.Header.Set("X-Initiator", "user")

	if baseURL != "" {
		base, err := url.Parse(baseURL)
		if err != nil {
			return nil, fmt.Errorf("parsing copilot base URL: %w", err)
		}

		cloned.URL.Scheme = base.Scheme
		cloned.URL.Host = base.Host
		cloned.Host = base.Host
	}

	return t.base.RoundTrip(cloned)
}
