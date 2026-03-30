// Package websearch provides a tool for searching the web via DuckDuckGo.
package websearch

import (
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/pkg/llm/tool"
)

const (
	userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	searchURL = "https://lite.duckduckgo.com/lite/"

	defaultResults = 5
	maxResults     = 20
	requestTimeout = 15 * time.Second
	minDelay       = 500 * time.Millisecond
	maxDelay       = 2 * time.Second
)

// Inputs defines the parameters for the WebSearch tool.
type Inputs struct {
	// Query is the search query string.
	Query string `json:"query" jsonschema:"required,description=Search query" validate:"required"`
	// MaxResults is the number of results to return (default 5, max 20).
	MaxResults int `json:"max_results,omitempty" jsonschema:"description=Number of results (default 5, max 20)"`
}

// result holds a single search result.
type result struct {
	Title   string
	URL     string
	Snippet string
}

// Tool implements web search via DuckDuckGo Lite.
type Tool struct {
	tool.Base

	mu         sync.Mutex
	lastSearch time.Time
}

// New creates a WebSearch tool.
func New() *Tool {
	return &Tool{
		Base: tool.Base{
			Text: tool.Text{
				Description: heredoc.Doc(`
					Searches the web using DuckDuckGo and returns a list of results.
					Each result includes a title, URL, and snippet.
					Use this to find documentation, error solutions, API references,
					changelogs, or any information beyond training data.
				`),
				Usage: heredoc.Doc(`
					Provide a search query. Optionally set max_results (default 5, max 20).
					Results are returned as a numbered list with title, URL, and snippet.
					Follow up with WebFetch to read the full content of a specific result.
				`),
				Examples: heredoc.Doc(`
					{"query": "golang html parser library"}
					{"query": "kubernetes pod crashloopbackoff fix", "max_results": 10}
					{"query": "react useEffect cleanup memory leak"}
				`),
			},
		},
	}
}

// Name returns the tool's identifier.
func (t *Tool) Name() string {
	return "WebSearch"
}

// Schema returns the provider-agnostic tool definition.
func (t *Tool) Schema() tool.Schema {
	return tool.GenerateSchema[Inputs](t)
}

// Sandboxable returns false because the tool makes network calls.
func (t *Tool) Sandboxable() bool {
	return false
}

// Execute performs a DuckDuckGo Lite search and returns formatted results.
func (t *Tool) Execute(ctx context.Context, args map[string]any) (string, error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return "", err
	}

	limit := params.MaxResults
	if limit <= 0 {
		limit = defaultResults
	}

	if limit > maxResults {
		limit = maxResults
	}

	t.rateLimit()

	results, err := t.search(ctx, params.Query, limit)
	if err != nil {
		return "", err
	}

	if len(results) == 0 {
		return fmt.Sprintf(
			"No results found for %q. DuckDuckGo may have returned a challenge page — try rephrasing or use WebFetch with a direct URL.",
			params.Query,
		), nil
	}

	return formatResults(params.Query, results), nil
}

// rateLimit enforces a random delay between consecutive searches.
// The lock is released during sleep so concurrent searches aren't serialized.
func (t *Tool) rateLimit() {
	t.mu.Lock()
	since := time.Since(t.lastSearch)
	t.mu.Unlock()

	if since < time.Second {
		delay := minDelay + time.Duration(rand.Int64N(int64(maxDelay-minDelay)))
		time.Sleep(delay)
	}

	t.mu.Lock()
	t.lastSearch = time.Now()
	t.mu.Unlock()
}

// search sends a POST request to DuckDuckGo Lite and parses the results.
func (t *Tool) search(ctx context.Context, query string, limit int) ([]result, error) {
	form := url.Values{"q": {query}}

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, searchURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, tool.NetworkError("DuckDuckGo", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, tool.HTTPError("DuckDuckGo", resp.StatusCode)
	}

	return parseResults(resp.Body, limit)
}

// parseResults extracts search results from DuckDuckGo Lite HTML.
//
// DuckDuckGo Lite structures results as table rows with:
//   - <a class="result-link"> for the title and URL
//   - <td class="result-snippet"> for the description snippet
func parseResults(r io.Reader, limit int) ([]result, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("parsing HTML: %w", err)
	}

	var results []result

	var current result

	var walk func(*html.Node)

	walk = func(n *html.Node) {
		if len(results) >= limit {
			return
		}

		if n.Type == html.ElementNode {
			switch {
			case n.Data == "a" && hasClass(n, "result-link"):
				current.Title = textContent(n)
				current.URL = attr(n, "href")

			case n.Data == "td" && hasClass(n, "result-snippet"):
				current.Snippet = strings.TrimSpace(textContent(n))

				if current.URL != "" {
					results = append(results, current)
					current = result{}
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}

	walk(doc)

	// Capture trailing result without snippet.
	if current.URL != "" && len(results) < limit {
		results = append(results, current)
	}

	return results, nil
}

// formatResults builds a numbered plain-text list.
func formatResults(query string, results []result) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Found %d results for %q:\n", len(results), query)

	for i, r := range results {
		fmt.Fprintf(&b, "\n%d. %s\n   URL: %s\n", i+1, r.Title, r.URL)

		if r.Snippet != "" {
			fmt.Fprintf(&b, "   %s\n", r.Snippet)
		}
	}

	return b.String()
}

// hasClass returns true if the node has the given CSS class.
func hasClass(n *html.Node, class string) bool {
	for _, a := range n.Attr {
		if a.Key == "class" {
			for _, c := range strings.Fields(a.Val) {
				if c == class {
					return true
				}
			}
		}
	}

	return false
}

// attr returns the value of the named attribute.
func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}

	return ""
}

// textContent extracts all text from a node tree.
func textContent(n *html.Node) string {
	var b strings.Builder

	var walk func(*html.Node)

	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}

	walk(n)

	return strings.TrimSpace(b.String())
}
