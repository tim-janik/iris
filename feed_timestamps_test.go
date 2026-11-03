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

func TestGenerateFeedsUsesLatestEntryChange(t *testing.T) {
	eng := mustTestEngine(t)
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	pubDate := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	modifiedDate := time.Date(2025, 7, 2, 3, 4, 5, 0, time.UTC)
	page := testInputPage(t, "2024/alpha.md", "Alpha", pageclass.PagePost)
	page.PubDate = pubDate
	page.ModDate = modifiedDate
	site := SiteConfig{URL: "https://example.com", Title: "Site", FeedAge: -1}
	siteGo := templates.SiteConfig{URL: site.URL, Title: site.Title, FeedAge: -1}
	outDir := t.TempDir()

	entries, err := generateFeeds(eng, []*InputPage{page}, site, siteGo, outDir, now)
	if err != nil {
		t.Fatal(err)
	}

	assertFileContains(t, filepath.Join(outDir, "rss2.xml"), "Wed, 02 Jul 2025 03:04:05 +0000")
	assertFileContains(t, filepath.Join(outDir, "atom.xml"), "2025-07-02T03:04:05Z")
	for _, entry := range entries {
		if entry.LastMod != "2025-07-02" {
			t.Errorf("feed sitemap lastmod = %q, want 2025-07-02", entry.LastMod)
		}
	}
}

func TestGenerateFeedsUsesPublicationWhenUnmodified(t *testing.T) {
	eng := mustTestEngine(t)
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	page := testInputPage(t, "2024/alpha.md", "Alpha", pageclass.PagePost)
	page.ModDate = time.Time{}
	site := SiteConfig{URL: "https://example.com", Title: "Site", FeedAge: -1}
	siteGo := templates.SiteConfig{URL: site.URL, Title: site.Title, FeedAge: -1}
	outDir := t.TempDir()

	if _, err := generateFeeds(eng, []*InputPage{page}, site, siteGo, outDir, now); err != nil {
		t.Fatal(err)
	}

	rss := readTestFile(t, filepath.Join(outDir, "rss2.xml"))
	if !strings.Contains(rss, "Wed, 01 May 2024 00:00:00 +0000") {
		t.Errorf("RSS feed did not use publication date:\n%s", rss)
	}
}
