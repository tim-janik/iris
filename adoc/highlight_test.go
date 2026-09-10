package adoc

import (
	"strings"
	"testing"
)

func TestRemoveHighlightScripts(t *testing.T) {
	tests := []struct {
		name   string
		script string
		marker string
		keep   bool
	}{
		{
			name:   "generated source",
			script: `<script src="assets/highlight.js//highlight.min.js"></script>`,
			marker: "assets/highlight.js//highlight.min.js",
		},
		{
			name:   "legacy initialization",
			script: `<script>if (!hljs.initHighlighting.called) { hljs.highlightBlock(el) }</script>`,
			marker: "hljs.highlightBlock",
		},
		{
			name:   "current initialization",
			script: `<script>hljs.initHighlighting()</script>`,
			marker: "hljs.initHighlighting()",
			keep:   true,
		},
		{
			name:   "metadata asset",
			script: `<script src="./..~meta~?asset=highlight.min.js"></script>`,
			marker: "./..~meta~?asset=highlight.min.js",
			keep:   true,
		},
		{
			name:   "unrelated script",
			script: `<script>keep()</script>`,
			marker: "keep()",
			keep:   true,
		},
	}
	var input strings.Builder
	input.WriteString("<html><body><p>body</p>")
	for _, test := range tests {
		input.WriteString(test.script)
	}
	input.WriteString("</body></html>")

	got := RemoveHighlightScripts(input.String())
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if present := strings.Contains(got, test.marker); present != test.keep {
				t.Errorf("marker %q present = %v, want %v: %s", test.marker, present, test.keep, got)
			}
		})
	}
}
