// This Source Code Form is licensed MPL-2.0: http://mozilla.org/MPL/2.0
package main

import (
	stdhtml "html"
	"net/url"
	"strings"
	"testing"

	"github.com/tim-janik/iris/pageclass"
	"github.com/tim-janik/iris/templates"
)

func TestRenderPageCommentLinkUsesConfiguredAddressAndEscaping(t *testing.T) {
	page := testInputPage(t, `notes/a"&%.md`, "Title", pageclass.PagePost)
	site := templates.SiteConfig{
		URL:           "https://example.com",
		Title:         "Site",
		CommentsEmail: "comments+%s@example.test",
	}
	data, err := renderPage(mustTestEngine(t), page, site)
	if err != nil {
		t.Fatal(err)
	}
	emailPath := `/notes/a"&%`
	commentText := "Add comment to " + emailPath
	query := url.Values{}
	query.Set("subject", commentText)
	query.Set("body", commentText+":\n\n")
	href := "mailto:comments+" + page.PageLUID() + "@example.test?" + strings.ReplaceAll(query.Encode(), "+", "%20")
	want := `href="` + strings.ReplaceAll(stdhtml.EscapeString(href), "+", "&#43;") + `"`
	if !strings.Contains(string(data), want) {
		t.Errorf("comment link missing or incorrectly escaped; wanted %s:\n%s", want, data)
	}
}

func TestRenderPageOmitsCommentLinkWhenDisabled(t *testing.T) {
	page := testInputPage(t, "notes/page.md", "Title", pageclass.PagePost)
	data, err := renderPage(mustTestEngine(t), page, templates.SiteConfig{URL: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "Post comment via email") {
		t.Errorf("comment action rendered with no configured email:\n%s", data)
	}
}
