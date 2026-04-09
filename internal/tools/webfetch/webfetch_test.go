package webfetch_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/tools/webfetch"
)

func TestExecuteMarkdown(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		_, _ = w.Write(
			[]byte(`<!DOCTYPE html><html><body><h1>Hello World</h1><p>Some paragraph text.</p></body></html>`),
		)
	}))
	defer srv.Close()

	tool := webfetch.New(5 * 1024 * 1024)

	result, err := tool.Execute(context.Background(), map[string]any{
		"url":    srv.URL,
		"format": "markdown",
	})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}

	if !strings.Contains(result, "Hello World") {
		t.Errorf("expected markdown output to contain heading text, got: %s", result)
	}

	// The HTML <h1> should become a markdown heading.
	if !strings.Contains(result, "#") {
		t.Errorf("expected markdown output to contain '#' heading marker, got: %s", result)
	}
}

func TestExecuteText(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		_, _ = w.Write(
			[]byte(`<!DOCTYPE html><html><body><h1>Plain heading</h1><p>Body content here.</p></body></html>`),
		)
	}))
	defer srv.Close()

	tool := webfetch.New(5 * 1024 * 1024)

	result, err := tool.Execute(context.Background(), map[string]any{
		"url":    srv.URL,
		"format": "text",
	})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}

	if !strings.Contains(result, "Plain heading") {
		t.Errorf("expected text output to contain heading text, got: %s", result)
	}

	if !strings.Contains(result, "Body content here") {
		t.Errorf("expected text output to contain paragraph text, got: %s", result)
	}

	// Plain text should not contain HTML tags.
	if strings.Contains(result, "<h1>") || strings.Contains(result, "<p>") {
		t.Errorf("expected plain text output to have no HTML tags, got: %s", result)
	}
}

func TestExecuteHTML(t *testing.T) {
	t.Parallel()

	rawHTML := `<!DOCTYPE html><html><body><h1>Raw HTML</h1></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		_, _ = w.Write([]byte(rawHTML))
	}))
	defer srv.Close()

	tool := webfetch.New(5 * 1024 * 1024)

	result, err := tool.Execute(context.Background(), map[string]any{
		"url":    srv.URL,
		"format": "html",
	})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}

	if !strings.Contains(result, "<h1>Raw HTML</h1>") {
		t.Errorf("expected html output to contain raw HTML tags, got: %s", result)
	}
}

func TestExecuteNon200(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	tool := webfetch.New(5 * 1024 * 1024)

	_, err := tool.Execute(context.Background(), map[string]any{
		"url": srv.URL,
	})
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}

	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected error to mention status code 404, got: %v", err)
	}
}

func TestExecuteInvalidScheme(t *testing.T) {
	t.Parallel()

	tool := webfetch.New(5 * 1024 * 1024)

	_, err := tool.Execute(context.Background(), map[string]any{
		"url": "ftp://example.com/file.txt",
	})
	if err == nil {
		t.Fatal("expected error for ftp:// scheme, got nil")
	}

	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("expected error to mention unsupported scheme, got: %v", err)
	}
}
