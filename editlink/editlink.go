// Package editlink provides "edit this heading" link injection for the iris serve
// HTTP server. Clicking an injected link opens the user's $EDITOR at the correct
// source line of the original markdown or asciidoc file.
package editlink

import (
	"fmt"
	"os"
	"strings"

	"github.com/tim-janik/iris/htmlutil"
	"golang.org/x/net/html"
)

// FindHeadingLine returns the 1-based line number of headingText in srcPath.
// occurrence is 0-indexed: 0 = first match, 1 = second match, etc.
//
// For .md files it looks for ATX headings (lines starting with #).
// For .adoc files it looks for section headings (lines starting with =).
func FindHeadingLine(srcPath string, headingText string, occurrence int) (int, error) {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return 0, err
	}

	text := string(data)
	lines := strings.Split(text, "\n")

	var marker rune
	switch {
	case strings.HasSuffix(srcPath, ".md"):
		marker = '#'
	case strings.HasSuffix(srcPath, ".adoc"):
		marker = '='
	default:
		// Default to markdown-style headings
		marker = '#'
	}

	markerStr := string(marker)
	matchCount := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, markerStr) {
			continue
		}
		stripped := strings.TrimLeft(trimmed, markerStr+" ")
		if strings.EqualFold(stripped, headingText) {
			if matchCount == occurrence {
				return i + 1, nil
			}
			matchCount++
		}
	}

	return 0, nil
}

// InjectEditLinks parses the HTML document, injects edit links next to each
// heading, and returns the modified HTML string.
func InjectEditLinks(htmlStr string, srcPath string) (string, error) {
	doc, err := htmlutil.Parse(htmlStr)
	if err != nil {
		return "", err
	}

	body := htmlutil.FindByTag(doc, "body")
	if body == nil {
		// No body — nothing to inject into
		return htmlStr, nil
	}

	// Collect all heading elements (h1 through h6) in document order
	headings := htmlutil.FindAll(doc, func(n *html.Node) bool {
		return n.Type == html.ElementNode && len(n.Data) == 2 &&
			n.Data[0] == 'h' && n.Data[1] >= '1' && n.Data[1] <= '6'
	})

	// Track per-heading-text occurrence counts for duplicate headings
	matchCount := make(map[string]int)

	for _, heading := range headings {
		headingText := htmlutil.Text(heading)
		if headingText == "" {
			continue
		}

		lineNum, err := FindHeadingLine(srcPath, headingText, matchCount[headingText])
		if err != nil || lineNum == 0 {
			continue
		}
		matchCount[headingText]++

		// Build edit link using javascript:void(fetch(...)) so the browser
		// does not navigate away from the page (same as the Python reference).
		jsHref := fmt.Sprintf("javascript:void(fetch('?edl=%d', {'cache':'no-cache'}))", lineNum)
		linkNode := &html.Node{
			Type: html.ElementNode,
			Data: "a",
			Attr: []html.Attribute{
				{Key: "class", Val: "iris-edit"},
				{Key: "href", Val: jsHref},
			},
		}
		linkNode.AppendChild(&html.Node{
			Type: html.TextNode,
			Data: "\u270e", // ✎ pencil character
		})

		// Append space separator, then the link
		heading.AppendChild(&html.Node{
			Type: html.TextNode,
			Data: " ",
		})
		heading.AppendChild(linkNode)
	}

	// Append global "edit document" link to body
	editDocLink := &html.Node{
		Type: html.ElementNode,
		Data: "a",
		Attr: []html.Attribute{
			{Key: "class", Val: "iris-edit"},
			{Key: "href", Val: "javascript:void(fetch('?edl=0', {'cache':'no-cache'}))"},
			{Key: "title", Val: "Edit Document [e]"},
			{Key: "accesskey", Val: "e"},
			{Key: "style", Val: "display:none"},
		},
	}
	body.AppendChild(editDocLink)

	// Inject CSS style block
	css := `.iris-edit { display: none; color: #888; transform: scaleX(-1); }
@media not print {
  h1:active .iris-edit, h1:hover .iris-edit,
  h2:active .iris-edit, h2:hover .iris-edit,
  h3:active .iris-edit, h3:hover .iris-edit,
  h4:active .iris-edit, h4:hover .iris-edit,
  h5:active .iris-edit, h5:hover .iris-edit,
  h6:active .iris-edit, h6:hover .iris-edit,
  .iris-edit:active,
  .iris-edit:hover { display: inline-block; }
}`
	styleNode := &html.Node{
		Type: html.ElementNode,
		Data: "style",
	}
	styleNode.AppendChild(&html.Node{
		Type: html.TextNode,
		Data: css,
	})
	body.AppendChild(styleNode)

	return htmlutil.Serialize(doc), nil
}
