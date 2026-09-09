package adoc

import (
	"strings"
	"testing"
)

func TestRemoveHighlightScripts(t *testing.T) {
	html := `<html><body><p>body</p><script src="assets/highlight.js//highlight.min.js"></script><script>if (!hljs.initHighlighting.called) { hljs.highlightBlock(el) }</script><script>keep()</script></body></html>`
	got := RemoveHighlightScripts(html)
	if strings.Contains(got, "highlight.min.js") || strings.Contains(got, "highlightBlock") || !strings.Contains(got, "keep()") {
		t.Fatalf("highlight scripts were not filtered: %s", got)
	}
}
