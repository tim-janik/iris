// Package adoc invokes asciidoctor to convert AsciiDoc to HTML.
package adoc

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/tim-janik/iris/htmlutil"
	"golang.org/x/net/html"
)

// Config holds asciidoctor invocation settings.
type Config struct {
	// Attributes passed to asciidoctor via -a flag
	Attributes []string
}

// DefaultConfig returns the default asciidoctor configuration matching the
// original Python implementation.
func DefaultConfig() Config {
	return Config{
		Attributes: []string{
			"xx=++",
			"Cxx=C++", "Cxx11=C++11", "Cxx14=C++14", "Cxx17=C++17",
			"skip-front-matter",
			"linkattrs",
			"sectanchors",
			"idprefix=",
			"idseparator=-",
			"linkcss",
			"stylesheet!",
			"stylesdir!",
			"webfonts!",
			"source-highlighter=highlightjs",
			"highlightjsdir=https://cdn.rawgit.com/tim-janik/cdn/a8a12dd652e48532faa39657bc5cc268525a89ca/highlightjs9/",
			"icons=font",
			"iconfont-cdn=https://cdnjs.cloudflare.com/ajax/libs/font-awesome/4.7.0/css/font-awesome.css",
		},
	}
}

// TitleFromSource extracts the page title from AsciiDoc source text the same
// way asciidoctor does: the first heading of any level (= Title, == Section,
// # Title, ## Section) becomes the document title, with inline formatting
// reduced to plaintext like asciidoctor's <title> output. Lines that are not
// headings (attribute entries, // comments, block delimiters such as a bare
// ====, prose) are skipped.
//
// This allows synthesizing a title without running asciidoctor, e.g. for
// dashboards that only parse frontmatter.
func TitleFromSource(data []byte) string {
	for _, line := range strings.Split(string(data), "\n") {
		if title, ok := headingText(line); ok {
			return plainText(title)
		}
	}
	return ""
}

// headingText returns the text of the heading on line, if line is a heading:
// "= " through "====== " (document title and section levels) or the
// single-line "# " form (##, ### ... map to deeper section levels). A bare
// delimiter line like "====" (example block) without trailing text is not a
// heading.
func headingText(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	n := 0
	for n < len(trimmed) && (trimmed[n] == '=' || trimmed[n] == '#') {
		n++
	}
	if n == 0 || n >= len(trimmed) || trimmed[n] != ' ' {
		return "", false
	}
	text := strings.TrimSpace(trimmed[n:])
	if text == "" {
		return "", false
	}
	return text, true
}

// plainText reduces AsciiDoc inline formatting to plain text, mirroring the
// text asciidoctor emits in <title>: code spans `x` / +x+, monospace #x#,
// bold *x*, italic _x_ (all constrained to word boundaries), and links
// url[Label] / <<id,Label>> collapse to their visible label.
func plainText(s string) string {
	s = reLink.ReplaceAllString(s, "$1")
	s = reXRef.ReplaceAllString(s, "$1")
	s = reCodeBacktick.ReplaceAllString(s, "$1")
	s = reCodePlus.ReplaceAllString(s, "$1$2$3")
	s = reMono.ReplaceAllString(s, "$1$2$3")
	s = reBold.ReplaceAllString(s, "$1$2$3")
	s = reItalic.ReplaceAllString(s, "$1$2$3")
	return s
}

var (
	// url[Label] or scheme:url[Label] → Label (https?://...; also bare word
	// followed by [Label] for link: and mailto: macros)
	reLink = regexp.MustCompile(`(?:https?|ftp|mailto):[^\s\[]+\[([^\]]*)\]`)
	// <<id,Label>> → Label
	reXRef = regexp.MustCompile(`<<[^,>]+,\s*([^>]+)>>`)
	// `code` → code
	reCodeBacktick = regexp.MustCompile("`([^`]+)`")
	// +code+ → code (word-boundary constrained so C++ and "a + b" survive)
	reCodePlus = regexp.MustCompile(`(^|[\s(])\+([^+]+)\+($|[\s).,;:!?])`)
	// #mono# → mono (word-boundary constrained)
	reMono = regexp.MustCompile(`(^|[\s(])#([^#\s]+)#($|[\s).,;:!?])`)
	// *bold* → bold, _italic_ → italic (word-boundary constrained)
	reBold   = regexp.MustCompile(`(^|[\s(])\*([^*]+)\*($|[\s).,;:!?])`)
	reItalic = regexp.MustCompile(`(^|[\s(])_([^_]+)_($|[\s).,;:!?])`)
)

// Result holds the disassembled HTML output from asciidoctor.
type Result struct {
	// Full standalone HTML document
	HTML string
	// Page title (from <title>)
	Title string
	// Body content (#content div)
	Content string
	// Header content (#header div)
	Header string
	// Footer text (#footer-text div)
	FooterUpdated string
	// Keywords from <meta name="keywords">
	Keywords []string
}

// Convert runs asciidoctor on the given adoc text and returns the full HTML.
func Convert(cfg Config, adocData []byte) (string, error) {
	// asciidoctor needs a file path, not stdin
	tmpFile, err := os.CreateTemp("", "adoc-*.adoc")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		tmpFile.Close()
		os.Remove(tmpPath)
	}()

	if _, err := tmpFile.Write(adocData); err != nil {
		return "", fmt.Errorf("write temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("close temp file: %w", err)
	}

	cmd := exec.Command("asciidoctor")
	for _, attr := range cfg.Attributes {
		cmd.Args = append(cmd.Args, "-a", attr)
	}
	cmd.Args = append(cmd.Args, tmpPath, "-o", "-")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("asciidoctor: %w: %s", err, stderr.String())
	}
	return stdout.String(), nil
}

// ConvertAndDisassemble runs asciidoctor and parses the output into structured fields.
func ConvertAndDisassemble(cfg Config, adoc []byte) (*Result, error) {
	htmlStr, err := Convert(cfg, adoc)
	if err != nil {
		return nil, err
	}

	r := &Result{HTML: htmlStr}
	doc, err := htmlutil.Parse(htmlStr)
	if err != nil {
		return nil, fmt.Errorf("parse HTML: %w", err)
	}

	r.Title = extractTitle(doc)
	r.Header = extractHeader(doc)
	r.Content = extractContent(doc)
	r.FooterUpdated = extractFooter(doc)
	r.Keywords = extractKeywords(doc)

	return r, nil
}

// extractTitle returns text from <title>.
func extractTitle(doc *html.Node) string {
	title := htmlutil.FindByTag(doc, "title")
	if title == nil {
		return ""
	}
	return htmlutil.Text(title)
}

// extractHeader returns inner HTML of <div id="header">.
func extractHeader(doc *html.Node) string {
	header := htmlutil.FindByID(doc, "header")
	if header == nil {
		return ""
	}
	return strings.TrimSpace(htmlutil.InnerHTML(header))
}

// extractContent returns inner HTML of <div id="content"> with first h1 stripped.
// Asciidoctor puts the page title in <div id="header"> and section headings
// start at <h2> in the content div. Only strip <h1> to avoid duplicating the
// title; <h2> elements are legitimate section headings and must be preserved.
func extractContent(doc *html.Node) string {
	content := htmlutil.FindByID(doc, "content")
	if content == nil {
		return ""
	}
	// Strip first <h1> if present (title duplicate), but never strip <h2>
	if h1 := htmlutil.FindByTag(content, "h1"); h1 != nil {
		htmlutil.Remove(h1)
	}
	return strings.TrimSpace(htmlutil.InnerHTML(content))
}

// extractFooter returns text from <div id="footer-text">.
func extractFooter(doc *html.Node) string {
	footer := htmlutil.FindByID(doc, "footer-text")
	if footer == nil {
		return ""
	}
	return htmlutil.Text(footer)
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
		// Pandoc/asciidoctor can embed newlines in the keywords meta content from YAML lists
		p = strings.Join(strings.Fields(p), " ")
		if p != "" {
			keywords = append(keywords, p)
		}
	}
	return keywords
}
