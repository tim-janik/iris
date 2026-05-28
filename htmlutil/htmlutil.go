// Package htmlutil provides HTML parsing utilities using golang.org/x/net/html.
package htmlutil

import (
	"strings"

	"golang.org/x/net/html"
)

// Parse parses the HTML document and returns the root node.
func Parse(raw string) (*html.Node, error) {
	return html.Parse(strings.NewReader(raw))
}

// Serialize renders the node tree back to an HTML string.
func Serialize(node *html.Node) string {
	var buf strings.Builder
	if err := html.Render(&buf, node); err != nil {
		return ""
	}
	return buf.String()
}

// InnerHTML returns the inner HTML of a node (children only, no self-closing tag).
func InnerHTML(node *html.Node) string {
	var buf strings.Builder
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		if err := html.Render(&buf, c); err != nil {
			return ""
		}
	}
	return buf.String()
}

// Text returns the concatenated text content of a node and its descendants.
func Text(node *html.Node) string {
	var buf strings.Builder
	collectText(node, &buf)
	return strings.TrimSpace(buf.String())
}

func collectText(node *html.Node, buf *strings.Builder) {
	if node.Type == html.TextNode {
		buf.WriteString(node.Data)
	}
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		collectText(c, buf)
	}
}

// FindByTag returns the first descendant element with the given tag name.
func FindByTag(node *html.Node, tag string) *html.Node {
	return findElement(node, func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == tag
	})
}

// FindByID returns the first descendant element with the given id attribute.
func FindByID(node *html.Node, id string) *html.Node {
	return findElement(node, func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return false
		}
		for _, a := range n.Attr {
			if a.Key == "id" && a.Val == id {
				return true
			}
		}
		return false
	})
}

// FindMeta returns the first <meta> element with the given name attribute.
func FindMeta(node *html.Node, name string) *html.Node {
	return findElement(node, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "meta" {
			return false
		}
		for _, a := range n.Attr {
			if a.Key == "name" && strings.EqualFold(a.Val, name) {
				return true
			}
		}
		return false
	})
}

// GetAttr returns the value of the named attribute, or empty string if not found.
func GetAttr(node *html.Node, key string) string {
	if node == nil {
		return ""
	}
	for _, a := range node.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// findElement walks the tree depth-first and returns the first node matching pred.
func findElement(node *html.Node, pred func(*html.Node) bool) *html.Node {
	if pred(node) {
		return node
	}
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		if found := findElement(c, pred); found != nil {
			return found
		}
	}
	return nil
}

// AppendChild adds child as the last child of parent.
func AppendChild(parent, child *html.Node) {
	if child.Parent != nil || child.PrevSibling != nil || child.NextSibling != nil {
		panic("htmlutil: AppendChild called for an attached child Node")
	}
	child.Parent = parent
	child.NextSibling = nil
	if last := parent.LastChild; last != nil {
		last.NextSibling = child
		child.PrevSibling = last
	} else {
		parent.FirstChild = child
	}
	parent.LastChild = child
}

// Remove removes a node from its parent.
func Remove(node *html.Node) {
	if node.Parent == nil {
		return
	}
	parent := node.Parent
	prev, next := node.PrevSibling, node.NextSibling
	if parent.FirstChild == node {
		parent.FirstChild = next
	}
	if parent.LastChild == node {
		parent.LastChild = prev
	}
	if prev != nil {
		prev.NextSibling = next
	}
	if next != nil {
		next.PrevSibling = prev
	}
	node.Parent = nil
	node.PrevSibling = nil
	node.NextSibling = nil
}

// RemoveAll removes all descendant elements matching pred.
// Walks bottom-up to avoid tree traversal issues during mutation.
func RemoveAll(node *html.Node, pred func(*html.Node) bool) {
	var toRemove []*html.Node
	collectMatching(node, pred, &toRemove)
	for _, n := range toRemove {
		Remove(n)
	}
}

func collectMatching(node *html.Node, pred func(*html.Node) bool, list *[]*html.Node) {
	if pred(node) {
		*list = append(*list, node)
	}
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		collectMatching(c, pred, list)
	}
}

// FindAll returns all descendant elements matching pred.
func FindAll(node *html.Node, pred func(*html.Node) bool) []*html.Node {
	var out []*html.Node
	collectAll(node, pred, &out)
	return out
}

func collectAll(node *html.Node, pred func(*html.Node) bool, list *[]*html.Node) {
	if pred(node) {
		*list = append(*list, node)
	}
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		collectAll(c, pred, list)
	}
}

// StripElements parses an HTML fragment, removes all elements whose tag name
// matches one of the given tags, and returns the serialized result.
// Useful for excluding <figure> blocks from excerpt text.
func StripElements(raw string, tags ...string) string {
	doc, err := Parse(raw)
	if err != nil {
		return raw
	}
	tagSet := make(map[string]struct{}, len(tags))
	for _, t := range tags {
		tagSet[t] = struct{}{}
	}
	var toRemove []*html.Node
	collectByTag(doc, tagSet, &toRemove)
	for _, n := range toRemove {
		Remove(n)
	}
	// Find the body or use the document root
	body := FindByTag(doc, "body")
	if body == nil {
		body = doc
	}
	return strings.TrimSpace(InnerHTML(body))
}

func collectByTag(node *html.Node, tags map[string]struct{}, list *[]*html.Node) {
	if _, ok := tags[node.Data]; ok && node.Type == html.ElementNode {
		*list = append(*list, node)
		return // don't recurse into removed nodes
	}
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		collectByTag(c, tags, list)
	}
}
