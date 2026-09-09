// This Source Code Form is licensed MPL-2.0: http://mozilla.org/MPL/2.0
package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/tim-janik/iris/pageclass"
)

func TestURLPathHelpersEscapeFilenamesAndJoinBases(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "fragment", path: "page#x", want: "page%23x"},
		{name: "query", path: "page?x", want: "page%3Fx"},
		{name: "percent", path: "100%", want: "100%25"},
		{name: "unicode", path: "über", want: "%C3%BCber"},
	}
	for _, test := range tests {
		if got := encodeURLPath(test.path); got != test.want {
			t.Errorf("encodeURLPath(%q) = %q, want %q", test.path, got, test.want)
		}
	}
	if got := joinURLPath("https://example.com/site/", "/page#x"); got != "https://example.com/site/page%23x" {
		t.Errorf("joinURLPath() = %q", got)
	}
	if got := joinURLPath("https://example.com/site", ""); got != "https://example.com/site/" {
		t.Errorf("joinURLPath() with empty path = %q", got)
	}
}

func TestNewFeedItemUsesEncodedAbsoluteURL(t *testing.T) {
	page := testInputPage(t, "notes/page#x.md", "Page", pageclass.PagePost)
	item := newFeedItem(page, "https://example.com/site/", "Site", 300)
	if item.URL != "https://example.com/site/notes/page%23x" {
		t.Errorf("feed item URL = %q", item.URL)
	}
	if item.LinkHref != "notes/page%23x" {
		t.Errorf("feed item link href = %q", item.LinkHref)
	}
}

func TestGenerateSitemapUsesEncodedURLPath(t *testing.T) {
	eng := mustTestEngine(t)
	page := testInputPage(t, "notes/page#x.md", "Page", pageclass.PagePost)
	outDir := t.TempDir()
	generateSitemap(eng, []*InputPage{page}, SiteConfig{URL: "https://example.com/site/"}, outDir, nil, page.ModDate)
	data := readTestFile(t, filepath.Join(outDir, "sitemap.xml"))
	if !strings.Contains(data, "https://example.com/site/notes/page%23x") {
		t.Errorf("sitemap URL was not encoded:\n%s", data)
	}
}
