package pandoc

import (
	"testing"

	"github.com/tim-janik/iris/htmlutil"
)

func TestDetectMermaid(t *testing.T) {
	tests := []struct {
		name string
		html string
		want bool
	}{
		{
			name: "fenced mermaid block",
			html: `<pre class="mermaid"><code>flowchart TD</code></pre>`,
			want: true,
		},
		{
			name: "plain code block",
			html: `<pre><code>flowchart TD</code></pre>`,
			want: false,
		},
		{
			name: "highlighted go block",
			html: `<div class="sourceCode"><pre class="sourceCode go"><code>func main()</code></pre></div>`,
			want: false,
		},
		{
			name: "mermaid mentioned in text",
			html: `<p>use mermaid for diagrams</p>`,
			want: false,
		},
	}
	for _, test := range tests {
		doc, err := htmlutil.Parse(test.html)
		if err != nil {
			t.Fatalf("%s: parse: %v", test.name, err)
		}
		if got := detectMermaid(bodyNode(doc)); got != test.want {
			t.Errorf("%s: detectMermaid() = %v, want %v", test.name, got, test.want)
		}
	}
}

func TestUnwrapMermaidCode(t *testing.T) {
	in := `<section><pre class="mermaid"><code>flowchart TD
  a --&gt; b</code></pre><pre><code>keep &lt;code&gt;</code></pre></section>`
	doc, err := htmlutil.Parse(in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	unwrapMermaidCode(bodyNode(doc))

	pre := htmlutil.FindByTag(bodyNode(doc), "pre")
	if pre == nil {
		t.Fatal("pre missing")
	}
	if got := htmlutil.Text(pre); got != "flowchart TD\n  a --> b" {
		t.Errorf("mermaid pre text = %q, want diagram source", got)
	}
	if htmlutil.FindByTag(pre, "code") != nil {
		t.Error("code element still nested in mermaid pre")
	}
	second := pre.NextSibling
	if second == nil || htmlutil.Text(second) != "keep <code>" {
		t.Errorf("plain code pre = %v, want untouched", second)
	}
}
