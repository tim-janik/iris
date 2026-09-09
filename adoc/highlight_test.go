package adoc

import (
	"strings"
	"testing"
)

func TestRemoveHighlightScripts(t *testing.T) {
	html := `<html><body><p>body</p><script src="assets/highlight.js//highlight.min.js"></script><script>if (!hljs.initHighlighting.called) { hljs.highlightBlock(el) }</script><script>hljs.initHighlighting()</script><script src="./..~meta~?asset=highlight.min.js"></script><script>keep()</script></body></html>`
	got := RemoveHighlightScripts(html)
	if strings.Contains(got, "assets/highlight.js//highlight.min.js") || strings.Contains(got, "highlightBlock") || !strings.Contains(got, "hljs.initHighlighting()") || !strings.Contains(got, "./..~meta~?asset=highlight.min.js") || !strings.Contains(got, "keep()") {
		t.Fatalf("highlight scripts were not filtered: %s", got)
	}
}
