package llamacpp_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/openai/openai-go/v3"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/providers"
	"github.com/idelchi/aura/pkg/providers/llamacpp"
	"github.com/idelchi/aura/pkg/providers/llamaswap"
)

// Responses observed in front of llama-swap: an authenticating proxy answers with an HTML
// login page, a stopped engine with llama.cpp's JSON error.
const (
	loginPage     = `<!DOCTYPE html><html><head><title>Login - Authelia</title></head><body><div id="root">` + "\n\n   Sign in   \n" + `</div></body></html>`
	engineStopped = `{"error":{"code":503,"message":"the engine is not running","type":"unavailable_error"}}`
	rateLimited   = `{"error":{"code":429,"message":"too many requests","type":"rate_limit_error"}}`
	contextFull   = `{"error":{"code":400,"message":"request (40000 tokens) exceeds the available context size (32768 tokens)","type":"exceed_context_size_error","n_prompt_tokens":40000,"n_ctx":32768}}`
)

// reply serves one fixed response on every path.
type reply struct {
	status      int
	contentType string
	body        string
	header      map[string]string
}

func serve(t *testing.T, r reply) string {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for k, v := range r.header {
			w.Header().Set(k, v)
		}

		w.Header().Set("Content-Type", r.contentType)
		w.WriteHeader(r.status)
		_, _ = w.Write([]byte(r.body))
	}))
	t.Cleanup(server.Close)

	return server.URL
}

// closedURL returns an address nothing listens on.
func closedURL(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	addr := listener.Addr().String()
	listener.Close()

	return "http://" + addr
}

func chat(t *testing.T, ctx context.Context, c *llamacpp.Client) error {
	t.Helper()

	_, _, err := c.Chat(ctx, request.Request{
		Model:    model.Model{Name: "m"},
		Messages: message.Messages{{Role: roles.User, Content: "hi"}},
	}, nil)

	return err
}

// expect checks the class, status, provider and diagnostic of a classified error.
func expect(t *testing.T, err error, class error, status int, provider string, diagnostic ...string) {
	t.Helper()

	if !errors.Is(err, class) {
		t.Fatalf("error %v is not %v", err, class)
	}

	got, ok := errors.AsType[*providers.StatusError](err)
	if !ok || got.Status != status || got.Provider != provider {
		t.Fatalf("status detail = %+v, want status %d from %s (%v)", got, status, provider, err)
	}

	for _, text := range diagnostic {
		if !strings.Contains(err.Error(), text) {
			t.Errorf("diagnostic %q missing from %v", text, err)
		}
	}
}

func TestMetadataErrorsAreClassified(t *testing.T) {
	for _, tc := range []struct {
		name       string
		reply      reply
		class      error
		diagnostic []string
	}{
		{"401 HTML", reply{401, "text/html; charset=utf-8", loginPage, nil}, providers.ErrAuth,
			[]string{"GET /props returned 401 Unauthorized", "text/html; charset=utf-8 body: <!DOCTYPE html>", "Sign in"}},
		{"403 HTML", reply{403, "text/html", loginPage, nil}, providers.ErrAuth, []string{"403 Forbidden"}},
		{"429 JSON", reply{429, "application/json", rateLimited, map[string]string{"Retry-After": "7"}},
			providers.ErrRateLimit, []string{"rate_limit_error: too many requests"}},
		{"503 engine stopped", reply{503, "application/json", engineStopped, nil}, providers.ErrServerError,
			[]string{"503 Service Unavailable: unavailable_error: the engine is not running"}},
		{"502 plain text", reply{502, "text/plain", "upstream connect error", nil}, providers.ErrServerError,
			[]string{"text/plain body: upstream connect error"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := llamacpp.New(serve(t, tc.reply), "", time.Minute).Model(t.Context(), "m")
			expect(t, err, tc.class, tc.reply.status, "llamacpp", tc.diagnostic...)

			if limited, ok := errors.AsType[*providers.RateLimitError](err); tc.reply.status == 429 && (!ok || limited.RetryAfter != 7*time.Second) {
				t.Errorf("retry hint = %+v, want 7s", limited)
			}
		})
	}
}

func TestDiagnosticsAreBounded(t *testing.T) {
	huge := "<html>" + strings.Repeat("<p>enormous login portal</p>", 10000) + "</html>"

	_, err := llamacpp.New(serve(t, reply{401, "text/html", huge, nil}), "", time.Minute).Model(t.Context(), "m")
	if !errors.Is(err, providers.ErrAuth) || len(err.Error()) > 600 {
		t.Fatalf("error of %d bytes: %.200v", len(err.Error()), err)
	}
}

func TestWrongSuccessBodyIsNotAnAuthFailure(t *testing.T) {
	// A proxy that answers 200 with its login page is not classified by guessing from prose.
	_, err := llamacpp.New(serve(t, reply{200, "text/html", loginPage, nil}), "", time.Minute).Model(t.Context(), "m")
	if err == nil || errors.Is(err, providers.ErrAuth) || !strings.Contains(err.Error(), "not the expected JSON") ||
		!strings.Contains(err.Error(), "text/html body:") {
		t.Fatalf("error = %v", err)
	}
}

func TestTransportFailuresAreNetworkErrors(t *testing.T) {
	_, err := llamacpp.New(closedURL(t), "", time.Minute).Model(t.Context(), "m")
	if !errors.Is(err, providers.ErrNetwork) {
		t.Fatalf("metadata error = %v, want a network error", err)
	}

	if err := chat(t, t.Context(), llamacpp.New(closedURL(t), "", time.Minute)); !errors.Is(err, providers.ErrNetwork) {
		t.Fatalf("inference error = %v, want a network error", err)
	}
}

func TestCancellationIsKept(t *testing.T) {
	blocked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(blocked.Close)

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()

	ctx, stop := context.WithCancel(ctx)
	time.AfterFunc(50*time.Millisecond, stop)

	_, err := llamacpp.New(blocked.URL, "", time.Minute).Model(ctx, "m")
	if !errors.Is(err, context.Canceled) || errors.Is(err, providers.ErrNetwork) {
		t.Fatalf("cancelled metadata request = %v, want only a cancellation", err)
	}
}

// The OpenAI SDK retries 429 and 5xx itself (honouring Retry-After) before failing.
func TestInferenceErrorsAreClassified(t *testing.T) {
	for _, tc := range []struct {
		name       string
		reply      reply
		class      error
		diagnostic string
	}{
		{"401 HTML", reply{401, "text/html", loginPage, nil}, providers.ErrAuth, "POST /v1/chat/completions returned 401 Unauthorized"},
		{"403 HTML", reply{403, "text/html", loginPage, nil}, providers.ErrAuth, "403 Forbidden"},
		{"429 JSON", reply{429, "application/json", rateLimited, map[string]string{"Retry-After": "1"}}, providers.ErrRateLimit, "too many requests"},
		{"503 engine stopped", reply{503, "application/json", engineStopped, nil}, providers.ErrServerError, "the engine is not running"},
		{"context full", reply{400, "application/json", contextFull, nil}, providers.ErrContextExhausted, "exceeds the available context size"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := chat(t, t.Context(), llamacpp.New(serve(t, tc.reply), "", time.Minute))
			expect(t, err, tc.class, tc.reply.status, "llamacpp", tc.diagnostic)

			if _, ok := errors.AsType[*openai.Error](err); !ok {
				t.Errorf("original SDK error lost: %v", err)
			}

			if limited, ok := errors.AsType[*providers.RateLimitError](err); tc.reply.status == 429 && (!ok || limited.RetryAfter != time.Second) {
				t.Errorf("retry hint = %+v, want 1s", limited)
			}
		})
	}
}

func TestLlamaSwapRoutesAreClassified(t *testing.T) {
	paths := make(chan string, 4)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.URL.Path

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(engineStopped))
	}))
	t.Cleanup(server.Close)

	client := llamaswap.New(server.URL, "", time.Minute)

	_, err := client.Model(t.Context(), "qwen")
	expect(t, err, providers.ErrServerError, 503, "llamaswap", "GET /upstream/qwen/props returned 503", "the engine is not running")

	err = client.UnloadModel(t.Context(), "qwen")
	expect(t, err, providers.ErrServerError, 503, "llamaswap", "POST /api/models/unload/qwen returned 503")

	if got := <-paths; got != "/upstream/qwen/props" {
		t.Errorf("metadata path = %s", got)
	}
}
