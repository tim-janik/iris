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
		name  string
		type_ pageclass.PageType
	}{
		{name: "root", type_: pageclass.PageTopIndex},
		{name: "nested", type_: pageclass.PageDirIndex},
	} {
		t.Run(test.name, func(t *testing.T) {
			page := testInputPage(t, test.name+"/index.md", "Index", test.type_)
			page.Rendered.Content = "<p>Authored index text</p>"
			if test.type_ == pageclass.PageTopIndex {
				page = testInputPage(t, "index.md", "Index", test.type_)
				page.Rendered.Content = "<p>Authored index text</p>"
			}
			html, err := renderPage(eng, page, templates.SiteConfig{Title: "Site"})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(html), "Authored index text") {
				t.Fatalf("rendered index omitted authored body: %s", html)
			}
		})
	}
}
