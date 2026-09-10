// This Source Code Form is licensed MPL-2.0: http://mozilla.org/MPL/2.0
package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tim-janik/iris/pageclass"
	"github.com/tim-janik/iris/templates"
)

func TestNewFeedItemUsesEncodedRelativeLink(t *testing.T) {
	page := testInputPage(t, "notes/page#x.md", "Page", pageclass.PagePost)
	item := newFeedItem(page, "https://example.com/site/", "Site", 300)
	if item.URL != "https://example.com/site/notes/page%23x" {
		t.Errorf("feed item URL = %q", item.URL)
	}
	if item.LinkHref != "notes/page%23x" {
		t.Errorf("feed item link href = %q", item.LinkHref)
	}
}

func TestGenerateFeedsKeepsRelativeEncodedAtomLink(t *testing.T) {
	eng := mustTestEngine(t)
	page := testInputPage(t, "notes/page#x.md", "Page", pageclass.PagePost)
	outDir := t.TempDir()
	site := SiteConfig{
		URL:     "https://example.com/site/",
		Title:   "Site",
		FeedURL: "http://localhost/feed.xml",
		FeedAge: -1,
	}
	siteGo := templates.SiteConfig{URL: site.URL, Title: site.Title, FeedAge: -1}
	if _, err := generateFeeds(eng, []*InputPage{page}, site, siteGo, outDir, time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	atom := readTestFile(t, filepath.Join(outDir, "atom.xml"))
	if !strings.Contains(atom, `<link rel="alternate" href="notes/page%23x"/>`) {
		t.Errorf("Atom link was not relative and encoded:\n%s", atom)
	}
}

func TestRenderPageCommentLinkEscapesFilenameURL(t *testing.T) {
	page := testInputPage(t, `notes/a#& %.md`, "Title", pageclass.PagePost)
	site := templates.SiteConfig{Title: "Site", CommentsEmail: "comments+%s@example.test"}
	data, err := renderPage(mustTestEngine(t), page, site)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "subject=Add%20comment%20to%20%2Fnotes%2Fa%23%26%20%25") {
		t.Errorf("comment link path was not escaped:\n%s", data)
	}
}
