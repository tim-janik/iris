// Package pandoc invokes the pandoc CLI to convert markdown to HTML.
package pandoc

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/tim-janik/iris/htmlutil"
	"golang.org/x/net/html"
)

// Config holds pandoc invocation settings.
type Config struct {
	// Markdown extensions (e.g., "markdown+autolink_bare_uris+emoji")
	InputFormat string
	// Extra pandoc flags
	ExtraArgs []string
}

// DefaultConfig returns the default pandoc configuration matching the
// original Python implementation.
func DefaultConfig() Config {
	return Config{
		InputFormat: "markdown+autolink_bare_uris+emoji+lists_without_preceding_blankline",
		ExtraArgs: []string{
			"--html-q-tags",
			"-s",
			"--section-divs",
			"--email-obfuscation=references",
			"--toc-depth=6",
		},
	}
}

// Result holds the disassembled HTML output from pandoc.
type Result struct {
	// Full standalone HTML document
	HTML string
	// First <h1> from body (used as page header)
	Header string
	// Body content without the first <h1>
	Content string
	// Page title (from <h1> or <title>)
	Title string
	// Keywords from <meta name="keywords">
	Keywords []string
	// Mermaid reports a fenced ```mermaid block, which needs mermaid.js
	// to render as a diagram
	Mermaid bool
}

// Convert runs pandoc on the given text and returns the full HTML.
// The inputFormat overrides cfg.InputFormat if non-empty.
func Convert(cfg Config, data []byte, inputFormat string) (string, error) {
	return ConvertWithTitle(cfg, data, inputFormat, "")
}

// ConvertWithTitle is like Convert, but passes a title to pandoc only when
// title is non-empty. The argument is passed directly (not through a shell),
// so source metadata cannot become command-line syntax.
func ConvertWithTitle(cfg Config, data []byte, inputFormat, title string) (string, error) {
	cmd := exec.Command("pandoc")
	inFmt := cfg.InputFormat
	if inputFormat != "" {
		inFmt = inputFormat
	}
	cmd.Args = append(cmd.Args, "-f", inFmt)
	cmd.Args = append(cmd.Args, cfg.ExtraArgs...)
	if title != "" {
		cmd.Args = append(cmd.Args, "--metadata", "title="+title)
	}
	cmd.Args = append(cmd.Args, "-o", "-")

	cmd.Stdin = bytes.NewReader(data)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pandoc: %w: %s", err, stderr.String())
	}
	return stdout.String(), nil
}

// ConvertAndDisassemble runs pandoc and parses the output into structured fields.
// The inputFormat overrides cfg.InputFormat if non-empty.
func ConvertAndDisassemble(cfg Config, data []byte, inputFormat string) (*Result, error) {
	return ConvertAndDisassembleWithTitle(cfg, data, inputFormat, "")
}

// ConvertAndDisassembleWithTitle is the title-aware disassembling variant.
func ConvertAndDisassembleWithTitle(cfg Config, data []byte, inputFormat, title string) (*Result, error) {
	htmlStr, err := ConvertWithTitle(cfg, data, inputFormat, title)
	if err != nil {
		return nil, err
	}

	r := &Result{HTML: htmlStr}
	doc, err := htmlutil.Parse(htmlStr)
	if err != nil {
		return nil, fmt.Errorf("parse HTML: %w", err)
	}
	unwrapMermaidCode(bodyNode(doc))

	r.Title = extractTitle(doc)
	r.Header = extractFirstH1(doc)
	r.Content = extractBodyWithoutFirstH1(doc)
	r.Keywords = extractKeywords(doc)
	r.Mermaid = detectMermaid(bodyNode(doc))

	return r, nil
}

// ExtractBodyAndTitle parses a full HTML document and returns the body's
// inner HTML (including any <h1>), the page title: the first <h1> wins,
// falling back to <title>; pandoc's "-" title placeholder counts as empty,
// and whether the body contains a mermaid diagram block.
// Shared by iris serve (which renders the body with its <h1> intact).
func ExtractBodyAndTitle(htmlStr string) (body, title string, mermaid bool) {
	doc, err := htmlutil.Parse(htmlStr)
	if err != nil {
		return htmlStr, "", false
	}
	unwrapMermaidCode(bodyNode(doc))
	body = strings.TrimSpace(htmlutil.InnerHTML(bodyNode(doc)))
	title = extractTitle(doc)
	if title == "-" {
		title = ""
	}
	mermaid = detectMermaid(bodyNode(doc))
	return body, title, mermaid
}

// bodyNode returns the <body> element, or the document root when absent
// (pandoc fragment output).
func bodyNode(doc *html.Node) *html.Node {
	if body := htmlutil.FindByTag(doc, "body"); body != nil {
		return body
	}
	return doc
}

// extractTitle returns text from first <h1> in body, fallback to <title>.
func extractTitle(doc *html.Node) string {
	if h1 := htmlutil.FindByTag(bodyNode(doc), "h1"); h1 != nil {
		return htmlutil.Text(h1)
	}
	if title := htmlutil.FindByTag(doc, "title"); title != nil {
		return htmlutil.Text(title)
	}
	return ""
}

// extractFirstH1 returns the full <h1> HTML element string.
func extractFirstH1(doc *html.Node) string {
	h1 := htmlutil.FindByTag(bodyNode(doc), "h1")
	if h1 == nil {
		return ""
	}
	return strings.TrimSpace(htmlutil.Serialize(h1))
}

// extractBodyWithoutFirstH1 extracts body content and removes the first <h1>.
func extractBodyWithoutFirstH1(doc *html.Node) string {
	// Remove first <h1>
	if h1 := htmlutil.FindByTag(bodyNode(doc), "h1"); h1 != nil {
		htmlutil.Remove(h1)
	}
	return strings.TrimSpace(htmlutil.InnerHTML(bodyNode(doc)))
}

// extractKeywords returns keywords from <meta name="keywords">.
func extractKeywords(doc *html.Node) []string {
	meta := htmlutil.FindMeta(doc, "keywords")
	if meta == nil {
		return nil
	}
	val := htmlutil.GetAttr(meta, "content")
	if val == "" {
		return nil
	}
	parts := strings.Split(val, ",")
	keywords := make([]string, 0, len(parts))
	for _, p := range parts {
		// Collapse internal whitespace (newlines, tabs, multiple spaces) into single spaces
		// Pandoc can embed newlines in the keywords meta content from YAML lists
		p = strings.Join(strings.Fields(p), " ")
		if p != "" {
			keywords = append(keywords, p)
		}
	}
	return keywords
}

// detectMermaid reports whether body contains an element with class
// "mermaid"; pandoc emits one <pre class="mermaid"> per fenced block.
func detectMermaid(body *html.Node) bool {
	found := htmlutil.FindAll(body, func(n *html.Node) bool {
		return n.Type == html.ElementNode && hasClass(n, "mermaid")
	})
	return len(found) > 0
}

// unwrapMermaidCode replaces <pre class="mermaid"><code>…</code></pre>
// with plain text content; mermaid's run() reads innerHTML, so nested
// markup would corrupt the diagram source.
func unwrapMermaidCode(body *html.Node) {
	for _, pre := range htmlutil.FindAll(body, func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == "pre" && hasClass(n, "mermaid")
	}) {
		text := htmlutil.Text(pre)
		for c := pre.FirstChild; c != nil; {
			next := c.NextSibling
			htmlutil.Remove(c)
			c = next
		}
		htmlutil.AppendChild(pre, &html.Node{Type: html.TextNode, Data: text})
	}
}

func hasClass(n *html.Node, class string) bool {
	for _, c := range strings.Fields(htmlutil.GetAttr(n, "class")) {
		if c == class {
			return true
		}
	}
	return false
}
