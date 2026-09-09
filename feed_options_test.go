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

func TestGenerateFeedsAppliesOptionsToItems(t *testing.T) {
	eng := mustTestEngine(t)
	outDir := t.TempDir()
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	page := testInputPage(t, "2026/alpha.md", "Alpha", pageclass.PagePost)
	page.Rendered.Content = "<p>full feed body</p>"
	site := SiteConfig{URL: "https://example.com", Title: "Site", TeaserLen: 300, FeedAge: -1}
	siteGo := templates.SiteConfig{URL: site.URL, Title: site.Title, FeedAge: -1, TeaserLen: site.TeaserLen}

	generateFeeds(eng, []*InputPage{page}, site, siteGo, outDir, now)

	atom := readTestFile(t, filepath.Join(outDir, "atom.xml"))
	if !strings.Contains(atom, `<content type="html"`) || !strings.Contains(atom, `full feed body`) {
		t.Errorf("Atom feed omitted full item content:\n%s", atom)
	}
	rss := readTestFile(t, filepath.Join(outDir, "rss2.xml"))
	if !strings.Contains(rss, `<description>full feed body</description>`) {
		t.Errorf("RSS feed omitted item description:\n%s", rss)
	}
	if strings.Contains(rss, `<content:encoded>`) {
		t.Errorf("RSS feed unexpectedly included full item content:\n%s", rss)
	}
}
