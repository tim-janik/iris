package adoc

import (
	"strings"
	"testing"
)

func TestConvertAndDisassembleKeepsFootnotes(t *testing.T) {
	result, err := ConvertAndDisassemble(DefaultConfig(), []byte("= Test\n\nA note footnote:[details].\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`id="footnotes"`, "details", `href="#_footnoteref_1"`, `id="_footnotedef_1"`} {
		if !strings.Contains(result.Content, want) {
			t.Fatalf("content omitted footnote definitions: %s", result.Content)
		}
	}
}
