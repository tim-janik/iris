// This Source Code Form is licensed MPL-2.0: http://mozilla.org/MPL/2.0
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tim-janik/iris/pageclass"
	"github.com/tim-janik/iris/pandoc"
	"github.com/tim-janik/iris/templates"
)

func mustTestEngine(t *testing.T) *templates.Engine {
	t.Helper()
	eng, err := templates.New("")
	if err != nil {
		t.Fatalf("templates.New(): %v", err)
	}
	return eng
}

// testInputPage builds an InputPage the way processSourceFile would for relPath.
func testInputPage(t *testing.T, relPath, title string, pgType pageclass.PageType) *InputPage {
	t.Helper()
	ext := filepath.Ext(relPath)
	dirName, depth, root := computePathInfo(relPath)
	return &InputPage{
		RelPath:    relPath,
		OutputPath: strings.TrimSuffix(relPath, ext) + ".html",
		DirName:    dirName,
		Stem:       strings.TrimSuffix(filepath.Base(relPath), ext),
		Type:       pgType,
		Depth:      depth,
		Root:       root,
		Front:      &Frontmatter{Title: title, Raw: map[string]string{}},
		Rendered:   &pandoc.Result{Content: "<p>" + title + " body</p>"},
		PubDate:    time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC),
		ModDate:    time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func assertFileContains(t *testing.T, path, substr string) {
	t.Helper()
	if s := readTestFile(t, path); !strings.Contains(s, substr) {
		t.Errorf("%s: expected to contain %q", path, substr)
	}
}

func assertFileNotContains(t *testing.T, path, substr string) {
	t.Helper()
	if s := readTestFile(t, path); strings.Contains(s, substr) {
		t.Errorf("%s: expected to NOT contain %q", path, substr)
	}
}

func TestSpecialScore(t *testing.T) {
	tests := []struct {
		loc  string
		want int
	}{
		{"https://example.com/", +10},
		{"https://example.com/index.html", +10},
		{"https://example.com/index.htm", +10},
		{"https://example.com/sitemap.xml", +10},
		{"/", +10},
		{"/index.html", +10},
		{"https://example.com/about", 0},
		{"https://example.com/blog/post", 0},
		{"https://example.com/rss2.xml", -3},
		{"https://example.com/atom.xml", -3},
		{"https://example.com/docs/index.htm", -2},
		{"https://example.com/google0a1b2c3d.html", -10},
	}
	for _, tt := range tests {
		if got := specialScore(tt.loc); got != tt.want {
			t.Errorf("specialScore(%q) = %d, want %d", tt.loc, got, tt.want)
		}
	}
}

// The auto dirindex for the root directory must not overwrite a root
// index.html rendered from a source page.
func TestGenerateDirIndices_RootIndexNotOverwritten(t *testing.T) {
	eng := mustTestEngine(t)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	outDir := t.TempDir()

	rootIndex := filepath.Join(outDir, "index.html")
	if err := os.WriteFile(rootIndex, []byte("RENDERED-ROOT-INDEX"), 0644); err != nil {
		t.Fatal(err)
	}

	pages := []*InputPage{
		testInputPage(t, "index.md", "Top Index", pageclass.PageTopIndex),
		testInputPage(t, "2024/alpha.md", "Alpha", pageclass.PagePost),
	}
	siteGo := templates.SiteConfig{URL: "https://example.com", Title: "Site"}
	entries := generateDirIndices(eng, pages, siteGo, outDir, now)

	if data := readTestFile(t, rootIndex); data != "RENDERED-ROOT-INDEX" {
		t.Errorf("root index.html was overwritten by the auto dirindex: %q", data)
	}
	for _, e := range entries {
		if e.Loc == "https://example.com/" {
			t.Errorf("unexpected sitemap entry for skipped root dirindex: %+v", e)
		}
	}
	assertFileContains(t, filepath.Join(outDir, "2024", "index.html"), "Alpha")
}

// Subdirectory dirindices list only posts living in exactly that directory;
// posts from descendant or sibling directories are excluded. The root
// dirindex lists all posts.
func TestGenerateDirIndices_ExactDirectoryFilter(t *testing.T) {
	eng := mustTestEngine(t)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	outDir := t.TempDir()

	pages := []*InputPage{
		testInputPage(t, "2024/alpha.md", "Alpha", pageclass.PagePost),
		testInputPage(t, "2024/nested/beta.md", "Beta", pageclass.PagePost),
		testInputPage(t, "2025/gamma.md", "Gamma", pageclass.PagePost),
	}
	siteGo := templates.SiteConfig{URL: "https://example.com", Title: "Site"}
	entries := generateDirIndices(eng, pages, siteGo, outDir, now)

	assertFileContains(t, filepath.Join(outDir, "2024", "index.html"), "Alpha")
	assertFileNotContains(t, filepath.Join(outDir, "2024", "index.html"), "Beta")
	assertFileNotContains(t, filepath.Join(outDir, "2024", "index.html"), "Gamma")

	assertFileContains(t, filepath.Join(outDir, "2024", "nested", "index.html"), "Beta")
	assertFileNotContains(t, filepath.Join(outDir, "2024", "nested", "index.html"), "Alpha")
	assertFileNotContains(t, filepath.Join(outDir, "2024", "nested", "index.html"), "Gamma")

	assertFileContains(t, filepath.Join(outDir, "2025", "index.html"), "Gamma")
	assertFileNotContains(t, filepath.Join(outDir, "2025", "index.html"), "Alpha")

	// One sitemap entry per generated dirindex (root, 2024, 2024/nested, 2025)
	wantLocs := []string{
		"https://example.com/",
		"https://example.com/2024/",
		"https://example.com/2024/nested/",
		"https://example.com/2025/",
	}
	if len(entries) != len(wantLocs) {
		t.Fatalf("expected %d sitemap entries, got %d: %+v", len(wantLocs), len(entries), entries)
	}
	for i, want := range wantLocs {
		if entries[i].Loc != want {
			t.Errorf("entries[%d].Loc = %q, want %q", i, entries[i].Loc, want)
		}
	}
	// The root dirindex gets the special-page score through its URL path
	if entries[0].Priority != "1.0" || entries[0].Changefreq != "always" {
		t.Errorf("root dirindex priority/changefreq = %q/%q, want 1.0/always", entries[0].Priority, entries[0].Changefreq)
	}
}

// Empty feeds must carry the build time instead of the zero time.
func TestGenerateFeeds_LastBuildFallbackToNow(t *testing.T) {
	eng := mustTestEngine(t)
	outDir := t.TempDir()
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	site := SiteConfig{URL: "https://example.com", Title: "Site"}
	siteGo := templates.SiteConfig{URL: site.URL, Title: site.Title}

	entries := generateFeeds(eng, nil, site, siteGo, outDir, now)

	for _, name := range []string{"rss2.xml", "atom.xml"} {
		out := readTestFile(t, filepath.Join(outDir, name))
		if strings.Contains(out, "0001") {
			t.Errorf("%s contains the zero-time date:\n%s", name, out)
		}
		if !strings.Contains(out, now.Format("2006")) {
			t.Errorf("%s does not contain the build year %d:\n%s", name, now.Year(), out)
		}
	}
	for _, e := range entries {
		if e.LastMod != now.Format(dateLayout) {
			t.Errorf("feed sitemap lastmod = %q, want %q", e.LastMod, now.Format(dateLayout))
		}
	}
}

// A configured feed_url flows into the feed self links and sitemap entries;
// without feed_url the conventional feed paths are used.
func TestGenerateFeeds_FeedURL(t *testing.T) {
	eng := mustTestEngine(t)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	pages := []*InputPage{
		testInputPage(t, "2024/alpha.md", "Alpha", pageclass.PagePost),
	}

	t.Run("configured feed_url", func(t *testing.T) {
		outDir := t.TempDir()
		site := SiteConfig{URL: "https://example.com", Title: "Site", FeedURL: "https://example.com/custom/feed.xml"}
		siteGo := templates.SiteConfig{URL: site.URL, Title: site.Title}
		entries := generateFeeds(eng, pages, site, siteGo, outDir, now)

		assertFileContains(t, filepath.Join(outDir, "rss2.xml"),
			`<atom:link rel="self" type="application/rss+xml" href="https://example.com/custom/feed.xml"/>`)
		assertFileContains(t, filepath.Join(outDir, "atom.xml"),
			`<id>https://example.com/custom/feed.xml</id>`)
		// Both feeds share the configured URL, so it is listed only once
		if len(entries) != 1 {
			t.Fatalf("expected 1 sitemap entry, got %d: %+v", len(entries), entries)
		}
		for _, e := range entries {
			if e.Loc != "https://example.com/custom/feed.xml" {
				t.Errorf("feed sitemap loc = %q, want the configured feed_url", e.Loc)
			}
		}
	})

	t.Run("default feed URLs", func(t *testing.T) {
		outDir := t.TempDir()
		site := SiteConfig{URL: "https://example.com", Title: "Site"}
		siteGo := templates.SiteConfig{URL: site.URL, Title: site.Title}
		entries := generateFeeds(eng, pages, site, siteGo, outDir, now)

		assertFileContains(t, filepath.Join(outDir, "rss2.xml"),
			`href="https://example.com/rss2.xml"`)
		assertFileContains(t, filepath.Join(outDir, "atom.xml"),
			`<id>https://example.com/atom.xml</id>`)
		wantLocs := map[string]bool{
			"https://example.com/rss2.xml": false,
			"https://example.com/atom.xml": false,
		}
		if len(entries) != len(wantLocs) {
			t.Fatalf("expected %d sitemap entries, got %d: %+v", len(wantLocs), len(entries), entries)
		}
		for _, e := range entries {
			if _, ok := wantLocs[e.Loc]; !ok {
				t.Errorf("unexpected feed sitemap loc %q", e.Loc)
			}
			wantLocs[e.Loc] = true
		}
		for loc, seen := range wantLocs {
			if !seen {
				t.Errorf("missing feed sitemap loc %q", loc)
			}
		}
	})
}

// Sitemap scoring compares URL paths, so special URLs match even when the
// site URL is non-empty.
func TestCalcChangefreqSpecialPaths(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	old := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

	if got := calcChangefreqForDate(old, "https://example.com/", now); got != "always" {
		t.Errorf("root changefreq = %q, want always", got)
	}
	if got := calcChangefreqForDate(old, "https://example.com/index.html", now); got != "always" {
		t.Errorf("index changefreq = %q, want always", got)
	}
	if got := calcChangefreqForDate(old, "https://example.com/rss2.xml", now); got != "weekly" {
		t.Errorf("feed changefreq = %q, want weekly", got)
	}
	if got := calcChangefreqForDate(old, "https://example.com/blog/post", now); got != "yearly" {
		t.Errorf("post changefreq = %q, want yearly", got)
	}
	if got := calcPriorityForPath("https://example.com/", 0); got != "1.0" {
		t.Errorf("root priority = %q, want 1.0", got)
	}
}
