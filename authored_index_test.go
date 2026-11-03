package main

import (
	"strings"
	"testing"

	"github.com/tim-janik/iris/pageclass"
	"github.com/tim-janik/iris/templates"
)

func TestAuthoredIndexContentIsRendered(t *testing.T) {
	eng := mustTestEngine(t)
	for _, test := range []struct {
		name   string
		pgType pageclass.PageType
	}{
		{name: "root", pgType: pageclass.PageTopIndex},
		{name: "nested", pgType: pageclass.PageDirIndex},
	} {
		t.Run(test.name, func(t *testing.T) {
			relPath := test.name + "/index.md"
			if test.pgType == pageclass.PageTopIndex {
				relPath = "index.md"
			}
			page := testInputPage(t, relPath, "Index", test.pgType)
			page.Rendered.Content = "<p>Authored index text</p>"
			html, err := renderPage(eng, page, templates.SiteConfig{Title: "Site"})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(html), "<p>Authored index text</p>") {
				t.Fatalf("rendered index omitted authored body: %s", html)
			}
		})
	}
}
