// Package codex implements the OpenAI device code flow for ChatGPT/Codex authentication.
package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/oauth2"

	"github.com/idelchi/aura/pkg/auth/devicecode"
)

const (
	clientID = "app_EMoamEEZ73f0CkXaXp7hrann"
	issuer   = "https://auth.openai.com"
)

var config = &oauth2.Config{
	ClientID:    clientID,
	RedirectURL: issuer + "/deviceauth/callback",
	Endpoint: oauth2.Endpoint{
		TokenURL: issuer + "/oauth/token",
	},
}

// Login runs the OpenAI device code flow and returns the refresh token (rt_...).
func Login(ctx context.Context) (string, error) {
	dc, err := requestDeviceCode(ctx)
	if err != nil {
		return "", fmt.Errorf("requesting device code: %w", err)
	}

	// Phase 1-2: Poll for authorization_code + code_verifier (non-standard OpenAI flow).
	var authCode, codeVerifier string

	_, err = devicecode.Run(ctx, devicecode.Config{
		UserCode:        dc.UserCode,
		VerificationURL: issuer + "/codex/device",
		Interval:        dc.Interval,
		ExpiresIn:       900,
		Poll: func(ctx context.Context) (string, error) {
			code, verifier, pollErr := pollOpenAI(ctx, dc.DeviceAuthID, dc.UserCode)
			if pollErr != nil {
				return "", pollErr
			}

			authCode = code
			codeVerifier = verifier

			return "ok", nil
		},
	})
	if err != nil {
		return "", err
	}

	// Phase 3: Exchange authorization_code for tokens (standard OAuth2 + PKCE).
	token, err := config.Exchange(ctx, authCode, oauth2.VerifierOption(codeVerifier))
	if err != nil {
		return "", fmt.Errorf("exchanging authorization code: %w", err)
	}

	refreshToken := token.RefreshToken
	if refreshToken == "" {
		return "", errors.New("no refresh token in response")
	}

	return refreshToken, nil
}

type deviceAuthResponse struct {
	DeviceAuthID string          `json:"device_auth_id"`
	UserCode     string          `json:"user_code"`
	RawInterval  json.RawMessage `json:"interval"`
	ExpiresAt    string          `json:"expires_at"`
	Interval     int             `json:"-"`
}

func requestDeviceCode(ctx context.Context) (*deviceAuthResponse, error) {
	body, _ := json.Marshal(map[string]string{"client_id": clientID})

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		issuer+"/api/accounts/deviceauth/usercode",
		strings.NewReader(string(body)),
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "aura/0.0.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, respBody)
	}

	var result deviceAuthResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	// interval may come as string or int
	if len(result.RawInterval) > 0 {
		s := strings.Trim(string(result.RawInterval), `"`)
		if v, err := strconv.Atoi(s); err == nil {
			result.Interval = v
		}
	}

	if result.Interval == 0 {
		result.Interval = 5
	}

	return &result, nil
}

func pollOpenAI(ctx context.Context, deviceAuthID, userCode string) (authCode, codeVerifier string, err error) {
	body, _ := json.Marshal(map[string]string{
		"device_auth_id": deviceAuthID,
		"user_code":      userCode,
	})

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		issuer+"/api/accounts/deviceauth/token",
		strings.NewReader(string(body)),
	)
	if err != nil {
		return "", "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "aura/0.0.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
		return "", "", devicecode.ErrPending
	}

	if resp.StatusCode != http.StatusOK {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", "", fmt.Errorf("reading response body: %w", err)
		}

		return "", "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, respBody)
	}

	var result struct {
		AuthorizationCode string `json:"authorization_code"`
		CodeVerifier      string `json:"code_verifier"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", err
	}

	return result.AuthorizationCode, result.CodeVerifier, nil
}
