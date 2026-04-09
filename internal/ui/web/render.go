package web

import (
	"bytes"
	"fmt"
	"html"
	"strings"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"

	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/part"
	"github.com/idelchi/aura/pkg/llm/tool/call"
)

// --- Goldmark + Chroma integration ---

// chromaRenderer renders fenced code blocks with chroma syntax highlighting.
type chromaRenderer struct {
	formatter *chromahtml.Formatter
	style     *chroma.Style
}

func (r *chromaRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, r.renderFencedCodeBlock)
}

func (r *chromaRenderer) renderFencedCodeBlock(
	w util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool,
) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	n := node.(*ast.FencedCodeBlock)

	// Extract language from info string.
	language := ""

	if n.Info != nil {
		info := string(n.Info.Segment.Value(source))
		if fields := strings.Fields(info); len(fields) > 0 {
			language = fields[0]
		}
	}

	// Extract code content.
	var code bytes.Buffer

	for i := 0; i < n.Lines().Len(); i++ {
		line := n.Lines().At(i)
		code.Write(line.Value(source))
	}

	// Get lexer.
	lexer := lexers.Get(language)
	if lexer == nil {
		lexer = lexers.Fallback
	}

	lexer = chroma.Coalesce(lexer)

	// Tokenize and format.
	tokens, err := lexer.Tokenise(nil, code.String())
	if err != nil {
		fmt.Fprintf(w, "<pre><code>%s</code></pre>", html.EscapeString(code.String()))

		return ast.WalkContinue, nil
	}

	var buf bytes.Buffer
	if err := r.formatter.Format(&buf, r.style, tokens); err != nil {
		fmt.Fprintf(w, "<pre><code>%s</code></pre>", html.EscapeString(code.String()))

		return ast.WalkContinue, nil
	}

	w.Write(buf.Bytes())

	return ast.WalkContinue, nil
}

// chromaExtension implements goldmark.Extender — adds chroma highlighting
// to fenced code blocks while preserving all default renderers.
type chromaExtension struct {
	formatter *chromahtml.Formatter
	style     *chroma.Style
}

func (e *chromaExtension) Extend(m goldmark.Markdown) {
	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(&chromaRenderer{
				formatter: e.formatter,
				style:     e.style,
			}, 100),
		),
	)
}

// newMarkdownRenderer creates a goldmark instance with chroma syntax highlighting.
func newMarkdownRenderer() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithExtensions(
			extension.NewTable(),
			extension.Strikethrough,
			extension.Linkify,
			&chromaExtension{
				formatter: chromahtml.New(
					chromahtml.WithClasses(true),
					chromahtml.WithLineNumbers(false),
				),
				style: styles.Get("monokai"),
			},
		),
	)
}

// renderMarkdown converts markdown source to HTML.
func renderMarkdown(md goldmark.Markdown, source string) string {
	var buf bytes.Buffer
	if err := md.Convert([]byte(source), &buf); err != nil {
		return fmt.Sprintf("<pre>%s</pre>", html.EscapeString(source))
	}

	return buf.String()
}

// highlightDiffHTML renders unified diff content as syntax-highlighted HTML with CSS classes.
func highlightDiffHTML(content string) string {
	lexer := lexers.Get("diff")
	if lexer == nil {
		return html.EscapeString(content)
	}

	lexer = chroma.Coalesce(lexer)

	iter, err := lexer.Tokenise(nil, content)
	if err != nil {
		return html.EscapeString(content)
	}

	formatter := chromahtml.New(chromahtml.WithClasses(true))

	var buf bytes.Buffer
	if err := formatter.Format(&buf, styles.Get("monokai"), iter); err != nil {
		return html.EscapeString(content)
	}

	return buf.String()
}

// generateChromaCSS returns the chroma CSS for the given style.
func generateChromaCSS() string {
	var buf bytes.Buffer

	formatter := chromahtml.New(chromahtml.WithClasses(true))
	formatter.WriteCSS(&buf, styles.Get("monokai"))

	return buf.String()
}

// --- HTML fragment builders ---

func renderStatus(s ui.Status, hints ui.DisplayHints) string {
	return fmt.Sprintf(`<span class="status-line">%s</span>`, html.EscapeString(s.StatusLine(hints)))
}

func renderMessageStarted(msgID string) string {
	return fmt.Sprintf(
		`<div class="message assistant" id="msg-%s"><div class="role">Aura</div><div class="parts" id="parts-%s"></div></div>`,
		msgID,
		msgID,
	)
}

func renderUserMessage(status ui.Status, text string) string {
	return fmt.Sprintf(
		`<div class="message user"><div class="role">%s</div><div class="parts"><p>%s</p></div></div>`,
		html.EscapeString(status.UserPrompt()),
		html.EscapeString(text),
	)
}

func renderPartAdded(msgID string, partIdx int, p part.Part) string {
	partID := fmt.Sprintf("part-%s-%d", msgID, partIdx)
	partsID := "parts-" + msgID

	switch p.Type {
	case part.Content:
		return fmt.Sprintf(
			`<div id="%s" hx-swap-oob="beforeend"><div class="content" id="%s"></div></div>`,
			partsID, partID,
		)
	case part.Thinking:
		return fmt.Sprintf(
			`<div id="%s" hx-swap-oob="beforeend"><details class="thinking" id="%s"><summary>Thinking...</summary><div class="thinking-content"></div></details></div>`,
			partsID,
			partID,
		)
	case part.Tool:
		var toolHTML string

		if p.Call != nil {
			header, args := p.Call.DisplayFull()

			toolHTML = fmt.Sprintf(
				`<div class="tool-header tool-pending">&#9675; %s</div>`,
				html.EscapeString(header),
			)

			if args != "" {
				toolHTML += fmt.Sprintf(`<pre class="tool-args">%s</pre>`, html.EscapeString(args))
			}
		}

		return fmt.Sprintf(
			`<div id="%s" hx-swap-oob="beforeend"><div class="tool-call" id="%s">%s</div></div>`,
			partsID, partID, toolHTML,
		)
	}

	return ""
}

func renderPartDelta(msgID string, partIdx int, delta string) string {
	partID := fmt.Sprintf("part-%s-%d", msgID, partIdx)

	return fmt.Sprintf(
		`<span id="%s" hx-swap-oob="beforeend">%s</span>`,
		partID, html.EscapeString(delta),
	)
}

func renderThinkingDelta(msgID string, partIdx int, delta string) string {
	partID := fmt.Sprintf("part-%s-%d", msgID, partIdx)

	// Append to the thinking-content div inside the details element.
	return fmt.Sprintf(
		`<div id="%s" hx-swap-oob="beforeend"><span>%s</span></div>`,
		partID, html.EscapeString(delta),
	)
}

func renderToolUpdate(msgID string, partIdx int, p part.Part) string {
	partID := fmt.Sprintf("part-%s-%d", msgID, partIdx)

	if p.Call == nil {
		return ""
	}

	var toolHTML string

	header, args := p.Call.DisplayFull()

	toolHTML = fmt.Sprintf(`<div class="tool-header">%s</div>`, html.EscapeString(header))

	if args != "" {
		toolHTML += fmt.Sprintf(`<pre class="tool-args">%s</pre>`, html.EscapeString(args))
	}

	switch p.Call.State {
	case call.Running:
		toolHTML += `<div class="tool-running">Running...</div>`
	case call.Complete:
		result := p.Call.DisplayResult()

		toolHTML += fmt.Sprintf(`<pre class="tool-result">%s</pre>`, html.EscapeString(result))
	case call.Error:
		result := p.Call.DisplayResult()

		toolHTML += fmt.Sprintf(`<pre class="tool-error">%s</pre>`, html.EscapeString(result))
	}

	return fmt.Sprintf(
		`<div id="%s" hx-swap-oob="innerHTML">%s</div>`,
		partID, toolHTML,
	)
}

func renderFinalized(msgID string, msg message.Message, md goldmark.Markdown) string {
	partsID := "parts-" + msgID

	var partsHTML strings.Builder

	for _, p := range msg.Parts {
		switch p.Type {
		case part.Content:
			rendered := renderMarkdown(md, p.Text)
			fmt.Fprintf(&partsHTML, `<div class="content rendered">%s</div>`, rendered)
		case part.Thinking:
			if p.Text != "" {
				fmt.Fprintf(
					&partsHTML,
					`<details class="thinking"><summary>Thinking...</summary><div class="thinking-content">%s</div></details>`,
					html.EscapeString(p.Text),
				)
			}
		case part.Tool:
			if p.Call != nil {
				header, args := p.Call.DisplayFull()

				partsHTML.WriteString(`<div class="tool-call">`)
				fmt.Fprintf(&partsHTML, `<div class="tool-header">%s</div>`, html.EscapeString(header))

				if args != "" {
					fmt.Fprintf(&partsHTML, `<pre class="tool-args">%s</pre>`, html.EscapeString(args))
				}

				switch p.Call.State {
				case call.Running:
					// Defensive: Running in finalized messages means interrupted execution.
					// Session normalization resets these to Pending, but handle gracefully.
					partsHTML.WriteString(`<div class="tool-running">Interrupted</div>`)
				case call.Complete:
					fmt.Fprintf(
						&partsHTML,
						`<pre class="tool-result">%s</pre>`,
						html.EscapeString(p.Call.DisplayResult()),
					)
				case call.Error:
					fmt.Fprintf(
						&partsHTML,
						`<pre class="tool-error">%s</pre>`,
						html.EscapeString(p.Call.DisplayResult()),
					)
				}

				partsHTML.WriteString(`</div>`)
			}
		}
	}

	if msg.Error != nil {
		fmt.Fprintf(&partsHTML, `<div class="error">Error: %s</div>`, html.EscapeString(msg.Error.Error()))
	}

	return fmt.Sprintf(`<div id="%s" hx-swap-oob="innerHTML">%s</div>`, partsID, partsHTML.String())
}

func renderAskDialog(e ui.AskRequired) string {
	var b strings.Builder

	b.WriteString(`<dialog open class="ask-dialog"><div class="dialog-content">`)
	fmt.Fprintf(&b, `<p class="dialog-question">%s</p>`, html.EscapeString(e.Question))

	for _, opt := range e.Options {
		desc := ""

		if opt.Description != "" {
			desc = fmt.Sprintf(` title="%s"`, html.EscapeString(opt.Description))
		}

		fmt.Fprintf(
			&b,
			`<button class="dialog-btn" hx-post="/ask" hx-vals='{"answer":"%s"}' hx-swap="none"%s>%s</button>`,
			html.EscapeString(opt.Label),
			desc,
			html.EscapeString(opt.Label),
		)
	}

	b.WriteString(`</div></dialog>`)

	return b.String()
}

func renderConfirmDialog(e ui.ToolConfirmRequired) string {
	var b strings.Builder

	b.WriteString(`<dialog open class="confirm-dialog"><div class="dialog-content">`)
	fmt.Fprintf(&b, `<p class="dialog-question">Confirm %s</p>`, html.EscapeString(e.ToolName))

	if e.Description != "" {
		fmt.Fprintf(&b, `<p class="dialog-description">%s</p>`, html.EscapeString(e.Description))
	}

	if e.DiffPreview != "" {
		fmt.Fprintf(&b, `<div class="diff-preview">%s</div>`, highlightDiffHTML(e.DiffPreview))
	} else if e.Detail != "" {
		fmt.Fprintf(&b, `<pre class="dialog-detail">%s</pre>`, html.EscapeString(e.Detail))
	}

	b.WriteString(
		`<button class="dialog-btn allow" hx-post="/confirm" hx-vals='{"action":"allow"}' hx-swap="none">Allow</button>`,
	)
	fmt.Fprintf(
		&b,
		`<button class="dialog-btn allow-session" hx-post="/confirm" hx-vals='{"action":"allow_session"}' hx-swap="none">Allow "%s" (session)</button>`,
		html.EscapeString(e.Pattern),
	)
	fmt.Fprintf(
		&b,
		`<button class="dialog-btn allow-project" hx-post="/confirm" hx-vals='{"action":"allow_project"}' hx-swap="none">Allow "%s" (project)</button>`,
		html.EscapeString(e.Pattern),
	)
	fmt.Fprintf(
		&b,
		`<button class="dialog-btn allow-global" hx-post="/confirm" hx-vals='{"action":"allow_global"}' hx-swap="none">Allow "%s" (global)</button>`,
		html.EscapeString(e.Pattern),
	)
	b.WriteString(
		`<button class="dialog-btn deny" hx-post="/confirm" hx-vals='{"action":"deny"}' hx-swap="none">Deny</button>`)

	b.WriteString(`</div></dialog>`)

	return b.String()
}

func renderCommandResult(e ui.CommandResult) string {
	var b strings.Builder

	b.WriteString(`<div class="command-result">`)

	if e.Command != "" {
		fmt.Fprintf(&b, `<div class="command-name">%s</div>`, html.EscapeString(e.Command))
	}

	if e.Error != nil {
		fmt.Fprintf(&b, `<div class="error">Error: %s</div>`, html.EscapeString(e.Error.Error()))
	} else if e.Message != "" {
		fmt.Fprintf(&b, `<div class="command-message">%s</div>`, html.EscapeString(e.Message))
	}

	b.WriteString(`</div>`)

	return b.String()
}

func renderSpinner(text string) string {
	if text == "" {
		return ""
	}

	return fmt.Sprintf(`<div class="spinner-text">%s</div>`, html.EscapeString(text))
}

func renderToolOutput(e ui.ToolOutputDelta) string {
	return fmt.Sprintf(`<div class="tool-live-output">%s</div>`, html.EscapeString(e.Line))
}

func renderAssistantDone() string {
	return `<span class="done" data-done="true"></span>`
}

func renderPickerAsText(e ui.PickerOpen) string {
	return renderCommandResult(ui.CommandResult{
		Message: e.Display(),
	})
}
