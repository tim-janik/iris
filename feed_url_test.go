// This Source Code Form is licensed MPL-2.0: http://mozilla.org/MPL/2.0
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tim-janik/iris/pageclass"
	"github.com/tim-janik/iris/templates"
)

func TestGenerateFeedsUseAbsoluteEncodedEntryLinks(t *testing.T) {
	eng := mustTestEngine(t)
	page := testInputPage(t, "posts/page#x.md", "Page", pageclass.PagePost)
	site := SiteConfig{
		URL:     "https://example.com/site/",
		Title:   "Site",
		FeedURL: "https://example.com/feeds/site.xml",
		FeedAge: -1,
	}
	siteGo := templates.SiteConfig{URL: site.URL, Title: site.Title, FeedAge: -1}
	outDir := t.TempDir()
	generateFeeds(eng, []*InputPage{page}, site, siteGo, outDir, time.Now())

	atom, err := os.ReadFile(filepath.Join(outDir, "atom.xml"))
	if err != nil {
		t.Fatal(err)
	}
	want := `<link rel="alternate" href="https://example.com/site/posts/page%23x"/>`
	if !strings.Contains(string(atom), want) {
		t.Errorf("Atom entry link = %q, want %q:\n%s", want, want, atom)
	}
}
