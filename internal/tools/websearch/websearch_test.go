package websearch

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// findElement does a depth-first search of the HTML tree rooted at n and
// returns the first ElementNode whose tag matches tag.
func findElement(n *html.Node, tag string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tag {
		return n
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findElement(c, tag); found != nil {
			return found
		}
	}

	return nil
}

func TestParseResultsBasic(t *testing.T) {
	t.Parallel()

	htmlStr := `<html><body><table>
		<tr><td><a class="result-link" href="https://example.com">Example Title</a></td></tr>
		<tr><td class="result-snippet">Example snippet text.</td></tr>
		<tr><td><a class="result-link" href="https://golang.org">Go Language</a></td></tr>
		<tr><td class="result-snippet">The Go programming language.</td></tr>
	</table></body></html>`

	results, err := parseResults(strings.NewReader(htmlStr), 10)
	if err != nil {
		t.Fatalf("parseResults returned unexpected error: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	if results[0].Title != "Example Title" {
		t.Errorf("results[0].Title = %q, want %q", results[0].Title, "Example Title")
	}

	if results[0].URL != "https://example.com" {
		t.Errorf("results[0].URL = %q, want %q", results[0].URL, "https://example.com")
	}

	if results[0].Snippet != "Example snippet text." {
		t.Errorf("results[0].Snippet = %q, want %q", results[0].Snippet, "Example snippet text.")
	}

	if results[1].Title != "Go Language" {
		t.Errorf("results[1].Title = %q, want %q", results[1].Title, "Go Language")
	}

	if results[1].URL != "https://golang.org" {
		t.Errorf("results[1].URL = %q, want %q", results[1].URL, "https://golang.org")
	}

	if results[1].Snippet != "The Go programming language." {
		t.Errorf("results[1].Snippet = %q, want %q", results[1].Snippet, "The Go programming language.")
	}
}

func TestParseResultsLimit(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	sb.WriteString("<html><body><table>")

	for i := 1; i <= 5; i++ {
		sb.WriteString(`<tr><td><a class="result-link" href="https://example.com/` +
			string(rune('0'+i)) + `">Title ` + string(rune('0'+i)) + `</a></td></tr>`)
		sb.WriteString(`<tr><td class="result-snippet">Snippet ` + string(rune('0'+i)) + `.</td></tr>`)
	}

	sb.WriteString("</table></body></html>")

	results, err := parseResults(strings.NewReader(sb.String()), 2)
	if err != nil {
		t.Fatalf("parseResults returned unexpected error: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results with limit=2, got %d", len(results))
	}
}

func TestParseResultsEmpty(t *testing.T) {
	t.Parallel()

	htmlStr := `<html><body></body></html>`

	results, err := parseResults(strings.NewReader(htmlStr), 10)
	if err != nil {
		t.Fatalf("parseResults returned unexpected error: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results for empty HTML, got %d", len(results))
	}
}

func TestFormatResults(t *testing.T) {
	t.Parallel()

	results := []result{
		{Title: "First Result", URL: "https://first.com", Snippet: "First snippet."},
		{Title: "Second Result", URL: "https://second.com", Snippet: "Second snippet."},
	}

	output := formatResults("my query", results)

	if !strings.HasPrefix(output, "Found 2 results for") {
		t.Errorf("output does not start with 'Found 2 results for', got: %q", output[:min(len(output), 40)])
	}

	if !strings.Contains(output, "1. First Result") {
		t.Errorf("output missing '1. First Result'; output = %q", output)
	}

	if !strings.Contains(output, "2. Second Result") {
		t.Errorf("output missing '2. Second Result'; output = %q", output)
	}

	if !strings.Contains(output, "https://first.com") {
		t.Errorf("output missing first URL; output = %q", output)
	}

	if !strings.Contains(output, "https://second.com") {
		t.Errorf("output missing second URL; output = %q", output)
	}
}

func TestFormatResultsEmpty(t *testing.T) {
	t.Parallel()

	output := formatResults("empty query", []result{})

	if !strings.HasPrefix(output, "Found 0 results for") {
		t.Errorf("output does not start with 'Found 0 results for', got: %q", output)
	}
}

func TestHasClass(t *testing.T) {
	t.Parallel()

	fragment := `<div class="result-link foo">text</div>`

	doc, err := html.Parse(strings.NewReader(fragment))
	if err != nil {
		t.Fatalf("html.Parse failed: %v", err)
	}

	node := findElement(doc, "div")
	if node == nil {
		t.Fatalf("could not find div element in parsed HTML")
	}

	if !hasClass(node, "result-link") {
		t.Errorf("hasClass(node, %q) = false, want true", "result-link")
	}

	if hasClass(node, "bar") {
		t.Errorf("hasClass(node, %q) = true, want false", "bar")
	}
}

func TestAttr(t *testing.T) {
	t.Parallel()

	fragment := `<a href="https://example.com">link</a>`

	doc, err := html.Parse(strings.NewReader(fragment))
	if err != nil {
		t.Fatalf("html.Parse failed: %v", err)
	}

	node := findElement(doc, "a")
	if node == nil {
		t.Fatalf("could not find a element in parsed HTML")
	}

	got := attr(node, "href")
	if got != "https://example.com" {
		t.Errorf("attr(node, %q) = %q, want %q", "href", got, "https://example.com")
	}
}

func TestTextContent(t *testing.T) {
	t.Parallel()

	fragment := `<div>hello <span>world</span></div>`

	doc, err := html.Parse(strings.NewReader(fragment))
	if err != nil {
		t.Fatalf("html.Parse failed: %v", err)
	}

	node := findElement(doc, "div")
	if node == nil {
		t.Fatalf("could not find div element in parsed HTML")
	}

	got := textContent(node)

	if !strings.Contains(got, "hello") {
		t.Errorf("textContent missing 'hello'; got %q", got)
	}

	if !strings.Contains(got, "world") {
		t.Errorf("textContent missing 'world'; got %q", got)
	}
}
