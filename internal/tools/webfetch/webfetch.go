// Package webfetch provides a tool for fetching web pages and converting them to markdown.
package webfetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"golang.org/x/net/html"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/MakeNowJust/heredoc/v2"
	"github.com/dustin/go-humanize"

	"github.com/idelchi/aura/pkg/llm/tool"
)

const (
	userAgent      = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	requestTimeout = 30 * time.Second
)

// noiseTags are HTML elements stripped before markdown conversion.
var noiseTags = []string{
	"script", "style", "nav", "header", "footer",
	"aside", "noscript", "iframe", "svg",
}

// collapseNewlines matches 3+ consecutive newlines.
var collapseNewlines = regexp.MustCompile(`\n{3,}`)

// Inputs defines the parameters for the WebFetch tool.
type Inputs struct {
	// URL is the web page to fetch.
	URL string `json:"url" jsonschema:"required,description=URL to fetch (http/https only)" validate:"required,url"`
	// Format is the output format: markdown (default), text, or html.
	Format string `json:"format,omitempty" jsonschema:"description=Output format: markdown (default) text or html,enum=markdown,enum=text,enum=html"`
}

// Tool implements web page fetching with HTML-to-markdown conversion.
type Tool struct {
	tool.Base

	maxBodySize int64
}

// New creates a WebFetch tool with the given body size limit.
func New(maxBodySize int64) *Tool {
	return &Tool{
		maxBodySize: maxBodySize,
		Base: tool.Base{
			Text: tool.Text{
				Description: heredoc.Doc(`
					Fetches a web page and returns its content as markdown, plain text, or raw HTML.
					Use this after WebSearch to read the full content of a specific result,
					or directly with a known URL for documentation, API references, etc.
				`),
				Usage: fmt.Sprintf(heredoc.Doc(`
					Provide a URL (http or https). Optionally set format:
					- "markdown" (default): HTML converted to clean markdown with noise removed
					- "text": plain text extracted from the page
					- "html": raw HTML body content
					Response is capped at %s. Oversized results are rejected by the tool guard.
				`), humanize.IBytes(uint64(maxBodySize))),
				Examples: heredoc.Doc(`
					{"url": "https://pkg.go.dev/net/http"}
					{"url": "https://example.com", "format": "text"}
					{"url": "https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API", "format": "markdown"}
				`),
			},
		},
	}
}

// Name returns the tool's identifier.
func (t *Tool) Name() string {
	return "WebFetch"
}

// Schema returns the provider-agnostic tool definition.
func (t *Tool) Schema() tool.Schema {
	return tool.GenerateSchema[Inputs](t)
}

// Sandboxable returns false because the tool makes network calls.
func (t *Tool) Sandboxable() bool {
	return false
}

// Execute fetches a URL and returns its content in the requested format.
func (t *Tool) Execute(ctx context.Context, args map[string]any) (string, error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return "", err
	}

	parsed, err := url.Parse(params.URL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported URL scheme %q — only http and https are allowed", parsed.Scheme)
	}

	format := params.Format
	if format == "" {
		format = "markdown"
	}

	body, contentType, err := t.fetch(ctx, params.URL)
	if err != nil {
		return "", err
	}

	content, err := convert(body, params.URL, format)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Fetched: %s (%s, %s chars)\n\n%s",
		params.URL,
		contentType,
		humanize.Comma(int64(len(content))),
		content,
	), nil
}

// fetch performs the HTTP GET and returns the body, content type, and any error.
func (t *Tool) fetch(ctx context.Context, rawURL string) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", tool.NetworkError(rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", tool.HTTPError(rawURL, resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, t.maxBodySize+1))
	if err != nil {
		return "", "", fmt.Errorf("reading response: %w", err)
	}

	if int64(len(data)) > t.maxBodySize {
		return "", "", fmt.Errorf("response body exceeds %s limit", humanize.IBytes(uint64(t.maxBodySize)))
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "unknown"
	}

	// Trim to just the media type.
	if idx := strings.IndexByte(ct, ';'); idx != -1 {
		ct = strings.TrimSpace(ct[:idx])
	}

	return string(data), ct, nil
}

// convert transforms raw HTML into the requested format.
func convert(body, rawURL, format string) (string, error) {
	switch format {
	case "html":
		return body, nil

	case "text":
		return extractText(body), nil

	case "markdown":
		return toMarkdown(body, rawURL)

	default:
		return "", fmt.Errorf("unknown format %q — use markdown, text, or html", format)
	}
}

// toMarkdown converts HTML to markdown with noise removal.
func toMarkdown(body, rawURL string) (string, error) {
	domain := ""

	if parsed, err := url.Parse(rawURL); err == nil {
		domain = parsed.Scheme + "://" + parsed.Host
	}

	conv := converter.NewConverter()

	// Remove noise elements.
	for _, tag := range noiseTags {
		conv.Register.TagType(tag, converter.TagTypeRemove, converter.PriorityStandard)
	}

	md, err := conv.ConvertString(body, converter.WithDomain(domain))
	if err != nil {
		// Fall back to html-to-markdown top-level function without custom config.
		md, err = htmltomarkdown.ConvertString(body)
		if err != nil {
			return "", fmt.Errorf("converting to markdown: %w", err)
		}
	}

	return collapseNewlines.ReplaceAllString(md, "\n\n"), nil
}

// extractText parses HTML and returns the text content of the <body>.
func extractText(body string) string {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return body
	}

	bodyNode := findBody(doc)
	if bodyNode == nil {
		bodyNode = doc
	}

	var b strings.Builder

	var walk func(*html.Node)

	walk = func(n *html.Node) {
		// Skip noise elements.
		if n.Type == html.ElementNode {
			if slices.Contains(noiseTags, n.Data) {
				return
			}
		}

		if n.Type == html.TextNode {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				if b.Len() > 0 {
					b.WriteByte(' ')
				}

				b.WriteString(text)
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}

	walk(bodyNode)

	return b.String()
}

// findBody locates the <body> element in the parse tree.
func findBody(n *html.Node) *html.Node {
	if n.Type == html.ElementNode && n.Data == "body" {
		return n
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findBody(c); found != nil {
			return found
		}
	}

	return nil
}
