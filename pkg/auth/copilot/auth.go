// Package copilot implements the GitHub device code flow for Copilot authentication.
package copilot

import (
	"context"
	"fmt"
	"os"

	"golang.org/x/oauth2"
)

const clientID = "Iv1.b507a08c87ecfe98"

var config = &oauth2.Config{
	ClientID: clientID,
	Scopes:   []string{"read:user"},
	Endpoint: oauth2.Endpoint{
		DeviceAuthURL: "https://github.com/login/device/code",
		TokenURL:      "https://github.com/login/oauth/access_token",
	},
}

// Login runs the GitHub device code flow and returns the GitHub OAuth token (ghu_...).
func Login(ctx context.Context) (string, error) {
	da, err := config.DeviceAuth(ctx)
	if err != nil {
		return "", fmt.Errorf("requesting device code: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\nEnter code: %s\n", da.UserCode)
	fmt.Fprintf(os.Stderr, "Open:       %s\n\n", da.VerificationURI)

	token, err := config.DeviceAccessToken(ctx, da)
	if err != nil {
		return "", fmt.Errorf("polling for token: %w", err)
	}

	return token.AccessToken, nil
}
