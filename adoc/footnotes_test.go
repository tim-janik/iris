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
	if !strings.Contains(result.Content, `id="footnotes"`) || !strings.Contains(result.Content, "details") {
		t.Fatalf("content omitted footnote definitions: %s", result.Content)
	}
}
