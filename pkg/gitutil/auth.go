package gitutil

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"

	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// authMethod is a named authentication strategy.
type authMethod struct {
	name string
	fn   func(repoURL string) (transport.AuthMethod, error)
}

func authChain(repoURL string) []authMethod {
	if isSSHURL(repoURL) {
		return []authMethod{
			{"SSH agent", sshAgentAuth},
			{"SSH key file", sshKeyFileAuth},
		}
	}

	return []authMethod{
		{"no auth (public)", noAuth},
		{"env token", envTokenAuth},
		{"git credential fill", gitCredentialFillAuth},
	}
}

func isSSHURL(u string) bool {
	return strings.HasPrefix(u, "git@") || strings.HasPrefix(u, "ssh://")
}

func noAuth(_ string) (transport.AuthMethod, error) {
	return nil, nil
}

func sshAgentAuth(_ string) (transport.AuthMethod, error) {
	if os.Getenv("SSH_AUTH_SOCK") == "" {
		return nil, errors.New("SSH_AUTH_SOCK not set")
	}

	auth, err := ssh.NewSSHAgentAuth("git")
	if err != nil {
		return nil, fmt.Errorf("ssh-agent: %w", err)
	}

	return auth, nil
}

func sshKeyFileAuth(_ string) (transport.AuthMethod, error) {
	home, err := folder.Home()
	if err != nil {
		return nil, nil
	}

	keyNames := []string{"id_ed25519", "id_rsa", "id_ecdsa"}

	for _, name := range keyNames {
		keyPath := home.Join(".ssh").WithFile(name).Path()
		if !file.New(keyPath).Exists() {
			continue
		}

		auth, err := ssh.NewPublicKeysFromFile("git", keyPath, "")
		if err != nil {
			continue
		}

		return auth, nil
	}

	return nil, errors.New("no SSH keys found in ~/.ssh/")
}

func gitCredentialFillAuth(repoURL string) (transport.AuthMethod, error) {
	if !file.New("git").InPath() {
		return nil, errors.New("git not found in PATH")
	}

	parsed, err := url.Parse(repoURL)
	if err != nil {
		return nil, fmt.Errorf("parsing URL: %w", err)
	}

	input := fmt.Sprintf("protocol=%s\nhost=%s\npath=%s\n",
		parsed.Scheme, parsed.Host, strings.TrimPrefix(parsed.Path, "/"))

	cmd := exec.Command("git", "credential", "fill")

	cmd.Stdin = strings.NewReader(input)

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git credential fill: %w (stderr: %s)", err, stderr.String())
	}

	creds := map[string]string{}

	for line := range strings.SplitSeq(stdout.String(), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			creds[k] = v
		}
	}

	user := creds["username"]
	pass := creds["password"]

	if user == "" && pass == "" {
		return nil, errors.New("git credential fill returned empty credentials")
	}

	return &http.BasicAuth{
		Username: user,
		Password: pass,
	}, nil
}

func envTokenAuth(_ string) (transport.AuthMethod, error) {
	user := os.Getenv("GIT_USERNAME")
	pass := os.Getenv("GIT_PASSWORD")

	if user == "" && pass == "" {
		for _, env := range []string{"GITLAB_TOKEN", "GITHUB_TOKEN", "GIT_TOKEN"} {
			if v := os.Getenv(env); v != "" {
				pass = v
				user = "oauth2"

				break
			}
		}
	}

	if user == "" && pass == "" {
		return nil, errors.New("no token found in GIT_USERNAME/GIT_PASSWORD, GITLAB_TOKEN, GITHUB_TOKEN, or GIT_TOKEN")
	}

	return &http.BasicAuth{
		Username: user,
		Password: pass,
	}, nil
}
