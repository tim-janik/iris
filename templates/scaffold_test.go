package templates

import (
	htmplt "html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEngineCreation(t *testing.T) {
	eng, err := New("")
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	if eng == nil {
		t.Fatal("expected non-nil engine")
	}
}

func TestRenderPage(t *testing.T) {
	eng := mustNewEngine(t)
	data := samplePageData()
	html, err := eng.RenderPage(data)
	if err != nil {
		t.Fatalf("RenderPage(): %v", err)
	}
	assertContains(t, string(html), "<!DOCTYPE html>")
	assertContains(t, string(html), "sitelogo")
	assertContains(t, string(html), "estbit")
	assertContains(t, string(html), "Hello World")
	assertContains(t, string(html), `body class="page"`)
	assertContains(t, string(html), `name="DC.modified"`)
	assertContains(t, string(html), "body.page h1")
	assertContains(t, string(html), "Last updated")
	// non-post pages should NOT have DC.date.issued
	assertNotContains(t, string(html), "DC.date.issued")
}

func TestRenderPageUsesLocalHighlightAssets(t *testing.T) {
	eng := mustNewEngine(t)
	html, err := eng.RenderPage(samplePageData())
	if err != nil {
		t.Fatalf("RenderPage(): %v", err)
	}
	out := string(html)
	assertContains(t, out, `href="../assets/highlight.js/styles/github.min.css"`)
	assertContains(t, out, `src="../assets/highlight.js/highlight.min.js"`)
	assertNotContains(t, out, "cdn.rawgit.com")
	assertNotContains(t, out, "cdnjs.cloudflare.com/ajax/libs/highlight.js")
}

func TestWriteAssets(t *testing.T) {
	dir := t.TempDir()
	if err := WriteAssets(dir, []byte("script"), []byte("style")); err != nil {
		t.Fatalf("WriteAssets(): %v", err)
	}
	for _, test := range []struct {
		name string
		want string
	}{
		{"assets/highlight.js/highlight.min.js", "script"},
		{"assets/highlight.js/styles/github.min.css", "style"},
	} {
		name := test.name
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if string(data) != test.want {
			t.Errorf("asset %s = %q, want %q", name, data, test.want)
		}
	}
}

func TestRenderPost(t *testing.T) {
	eng := mustNewEngine(t)
	data := samplePageData()
	data.Page.IsPost = true
	data.Comments = []htmplt.HTML{`<p>Great post!</p>`}
	html, err := eng.RenderPost(data)
	if err != nil {
		t.Fatalf("RenderPost(): %v", err)
	}
	assertContains(t, string(html), `body class="post"`)
	assertContains(t, string(html), "Great post!")
	assertContains(t, string(html), "commentsect")
	assertContains(t, string(html), `name="keywords"`)
	assertContains(t, string(html), `name="DC.date.issued"`)
	assertContains(t, string(html), "citation_author")
}

func TestRenderDirIndex(t *testing.T) {
	eng := mustNewEngine(t)
	data := samplePageData()
	data.FeedItems = sampleFeedItems()
	data.ShowIndexTitle = true
	html, err := eng.RenderDirIndex(data)
	if err != nil {
		t.Fatalf("RenderDirIndex(): %v", err)
	}
	assertContains(t, string(html), `body class="dirindex"`)
	assertContains(t, string(html), "sitenav")
	assertContains(t, string(html), "postlist")
	assertContains(t, string(html), "<h2>posts</h2>")
	// dirindex should NOT have DC.modified
	assertNotContains(t, string(html), "DC.modified")
}

func TestRenderRSS(t *testing.T) {
	eng := mustNewEngine(t)
	data := sampleFeedData()
	xml, err := eng.RenderRSS(data)
	if err != nil {
		t.Fatalf("RenderRSS(): %v", err)
	}
	assertContains(t, string(xml), `<rss version="2.0"`)
	assertContains(t, string(xml), "<channel>")
	assertContains(t, string(xml), "<item>")
	assertContains(t, string(xml), "<pubDate>")
	assertContains(t, string(xml), `<guid isPermaLink="true">`)
	assertContains(t, string(xml), "<category>")
	assertContains(t, string(xml), "<lastBuildDate>")
}

func TestRenderAtom(t *testing.T) {
	eng := mustNewEngine(t)
	data := sampleFeedData()
	xml, err := eng.RenderAtom(data)
	if err != nil {
		t.Fatalf("RenderAtom(): %v", err)
	}
	assertContains(t, string(xml), "<feed")
	assertContains(t, string(xml), "<entry>")
	assertContains(t, string(xml), "<updated>")
	assertContains(t, string(xml), "<published>")
	assertContains(t, string(xml), `<category term=`)
	assertContains(t, string(xml), "<summary>")
	assertContains(t, string(xml), `<content type="html"`)
}

func TestRenderSitemap(t *testing.T) {
	eng := mustNewEngine(t)
	data := SitemapData{
		Pages: []SitemapEntry{
			{Loc: "https://example.com/page", Priority: "0.9", Changefreq: "weekly", LastMod: "2024-01-15"},
		},
	}
	xml, err := eng.RenderSitemap(data)
	if err != nil {
		t.Fatalf("RenderSitemap(): %v", err)
	}
	assertContains(t, string(xml), `<urlset`)
	assertContains(t, string(xml), `<loc>https://example.com/page</loc>`)
}

func TestRenderSitemapElementOrder(t *testing.T) {
	eng := mustNewEngine(t)
	data := SitemapData{
		Pages: []SitemapEntry{{
			Loc:        "https://example.com/page",
			LastMod:    "2024-01-15",
			Changefreq: "weekly",
			Priority:   "0.9",
		}},
	}
	xml, err := eng.RenderSitemap(data)
	if err != nil {
		t.Fatalf("RenderSitemap(): %v", err)
	}
	out := string(xml)
	want := "<loc>https://example.com/page</loc>\n    <lastmod>2024-01-15</lastmod>\n    <changefreq>weekly</changefreq>\n    <priority>0.9</priority>"
	if !strings.Contains(out, want) {
		t.Fatalf("sitemap elements are out of order:\n%s", out)
	}
}

func TestBuildFeedItems(t *testing.T) {
	pages := []PageData{
		{Title: "Post A", IsPost: true, PublishedDate: time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)},
		{Title: "Post B", IsPost: true, PublishedDate: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)},
		{Title: "Page C", IsPost: false, PublishedDate: time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC)},
	}
	site := SiteConfig{Title: "Test Site"}
	items := BuildFeedItems(pages, site, "", FeedOptions{})

	if len(items) != 2 {
		t.Fatalf("expected 2 feed items, got %d", len(items))
	}
	// Should be sorted newest first
	if items[0].Title != "Post B" {
		t.Errorf("expected first item to be 'Post B', got '%s'", items[0].Title)
	}
}

// Subdirectory listings include only posts living in exactly that directory;
// posts from descendant directories are excluded. The root directory lists
// all posts.
func TestBuildFeedItems_ExactDirectoryFilter(t *testing.T) {
	pages := []PageData{
		{Title: "In 2024", IsPost: true, DirName: "/2024/", Stem: "in-2024", PublishedDate: time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)},
		{Title: "Nested", IsPost: true, DirName: "/2024/nested/", Stem: "nested", PublishedDate: time.Date(2024, 1, 12, 0, 0, 0, 0, time.UTC)},
		{Title: "In 2025", IsPost: true, DirName: "/2025", Stem: "in-2025", PublishedDate: time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC)},
	}
	site := SiteConfig{Title: "Test Site"}

	for _, baseDir := range []string{"", "/"} {
		if got := BuildFeedItems(pages, site, baseDir, FeedOptions{}); len(got) != 3 {
			t.Errorf("baseDir %q: expected all 3 posts, got %d", baseDir, len(got))
		}
	}
	got := BuildFeedItems(pages, site, "/2024", FeedOptions{})
	if len(got) != 1 || got[0].Title != "In 2024" {
		t.Errorf("baseDir /2024: expected only 'In 2024', got %+v", got)
	}
	got = BuildFeedItems(pages, site, "/2024/nested", FeedOptions{})
	if len(got) != 1 || got[0].Title != "Nested" {
		t.Errorf("baseDir /2024/nested: expected only 'Nested', got %+v", got)
	}
	got = BuildFeedItems(pages, site, "/2025", FeedOptions{})
	if len(got) != 1 || got[0].Title != "In 2025" {
		t.Errorf("baseDir /2025: expected only 'In 2025', got %+v", got)
	}
}

// --- helpers ---

func mustNewEngine(t *testing.T) *Engine {
	eng, err := New("")
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	return eng
}

func samplePageData() TemplateData {
	now := time.Now()
	return TemplateData{
		Site: SiteConfig{
			Title:       "Testbit",
			Slogan:      "Bits and Pieces",
			Description: "A collection of projects.",
			URL:         "https://testbit.eu/",
			Authors:     []string{"Tim Janik"},
			Copyright:   "Copyright 2024",
			FeedURL:     "https://testbit.eu/feed.xml",
			IconHref:    "/favicon.ico",
		},
		Page: PageData{
			Title:         "Hello World",
			Content:       `<p>Hello, world!</p>`,
			FooterUpdated: "Last updated 2024-01-15",
			Keywords:      []string{"intro", "hello"},
			Authors:       []string{"Tim Janik"},
			DirName:       "posts",
			Stem:          "hello-world",
			Depth:         1,
			IsPost:        false,
			PublishedDate: now.AddDate(0, 0, -30),
			ModifiedDate:  now,
			LUID:          "abc123",
		},
		Root: "..",
	}
}

func sampleFeedItems() []FeedItem {
	return []FeedItem{
		{
			Title:         "Hello World",
			URL:           "https://testbit.eu/posts/hello-world",
			LinkHref:      "posts/hello-world",
			PublishedDate: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			ModifiedDate:  time.Now(),
			Keywords:      []string{"intro", "hello", "world"},
			Excerpt:       "First post on the new site.",
			SiteTitle:     "Testbit",
		},
	}
}

func sampleFeedData() FeedData {
	return FeedData{
		Site: SiteConfig{
			Title:     "Testbit",
			Slogan:    "Bits and Pieces",
			URL:       "https://testbit.eu/",
			Authors:   []string{"Tim Janik"},
			Copyright: "Copyright 2024",
		},
		FeedURL: "https://testbit.eu/atom.xml",
		Items: []FeedItem{
			{
				Title:           "Hello World",
				URL:             "https://testbit.eu/posts/hello-world",
				PublishedDate:   time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
				ModifiedDate:    time.Now(),
				Keywords:        []string{"intro"},
				Excerpt:         "First post.",
				FullContent:     `<p>Hello, world!</p>`,
				SiteTitle:       "Testbit",
				Options: FeedOptions{WithDescription: true, WithContent: true},
			},
		},
		LastBuild: time.Now(),
	}
}

func TestRenderRSS_URLAndAttributeEscaping(t *testing.T) {
	eng := mustNewEngine(t)
	now := time.Now()
	data := FeedData{
		Site: SiteConfig{
			Title:    "Test Site",
			Slogan:   "A tagline",
			URL:      "https://example.com/",
			Authors:  []string{"Author"},
			LogoHref: "/logo.png",
		},
		FeedURL: "https://example.com/feed?type=rss&v=1",
		Items: []FeedItem{
			{
				Title:         "Post",
				URL:           "https://example.com/post?id=1&cat=2",
				PublishedDate: now,
				ModifiedDate:  now,
				SiteTitle:     "Test Site",
				Options:       FeedOptions{WithDescription: true},
				Excerpt:       "An excerpt.",
			},
		},
		LastBuild: now,
	}
	xml, err := eng.RenderRSS(data)
	if err != nil {
		t.Fatalf("RenderRSS(): %v", err)
	}
	out := string(xml)

	// Channel-level URLs should be escaped
	assertContains(t, out, `href="https://example.com/feed?type=rss&amp;v=1"`)
	assertContains(t, out, "<link>https://example.com/</link>")

	// Image block should be escaped
	assertContains(t, out, "<url>https://example.com/logo.png</url>")

	// Item link and guid should be escaped
	assertContains(t, out, "<link>https://example.com/post?id=1&amp;cat=2</link>")
	assertContains(t, out, `<guid isPermaLink="true">https://example.com/post?id=1&amp;cat=2</guid>`)

	// source url attribute should be escaped
	assertContains(t, out, `url="https://example.com/feed?type=rss&amp;v=1"`)
}

func TestRenderAtom_URLAndAttributeEscaping(t *testing.T) {
	eng := mustNewEngine(t)
	now := time.Now()
	data := FeedData{
		Site: SiteConfig{
			Title:     "Test Site",
			Slogan:    "A tagline",
			URL:       "https://example.com/",
			Authors:   []string{"Author"},
			IconHref:  "/favicon.ico",
			LogoHref:  "/logo.png",
		},
		FeedURL: "https://example.com/feed?type=atom&v=1",
		Items: []FeedItem{
			{
				Title:         "Post",
				URL:           "https://example.com/post?id=1&cat=2",
				LinkHref:      "post?id=1&cat=2",
				PublishedDate: now,
				ModifiedDate:  now,
				SiteTitle:     "Test Site",
				Options:       FeedOptions{WithDescription: true},
				Excerpt:       "An excerpt.",
			},
		},
		LastBuild: now,
	}
	xml, err := eng.RenderAtom(data)
	if err != nil {
		t.Fatalf("RenderAtom(): %v", err)
	}
	out := string(xml)

	// Feed-level URLs should be escaped
	assertContains(t, out, `<id>https://example.com/feed?type=atom&amp;v=1</id>`)
	assertContains(t, out, `href="https://example.com/feed?type=atom&amp;v=1"`)
	assertContains(t, out, `href="https://example.com/"`)

	// Icon and logo should be escaped
	assertContains(t, out, "<icon>https://example.com/favicon.ico</icon>")
	assertContains(t, out, "<logo>https://example.com/logo.png</logo>")

	// Entry id and link should be escaped
	assertContains(t, out, `<id>https://example.com/post?id=1&amp;cat=2</id>`)
	assertContains(t, out, `href="post?id=1&amp;cat=2"`)
}

func TestRenderSitemap_XMLEscaping(t *testing.T) {
	eng := mustNewEngine(t)
	data := SitemapData{
		Pages: []SitemapEntry{
			{
				Loc:        "https://example.com/page?foo=1&bar=2",
				Priority:   "0.9",
				Changefreq: "weekly",
				LastMod:    "2024-01-15",
			},
		},
	}
	xml, err := eng.RenderSitemap(data)
	if err != nil {
		t.Fatalf("RenderSitemap(): %v", err)
	}
	out := string(xml)

	// URL with & should have &amp; in the output
	assertContains(t, out, `<loc>https://example.com/page?foo=1&amp;bar=2</loc>`)
}

func TestRenderRSS_XMLEscaping(t *testing.T) {
	eng := mustNewEngine(t)
	now := time.Now()
	data := FeedData{
		Site: SiteConfig{
			Title:   "Site & \"Title\" <with> specials",
			Slogan:  "A & B",
			URL:     "https://example.com/",
			Authors: []string{"Author & Co"},
		},
		FeedURL: "https://example.com/rss2.xml",
		Items: []FeedItem{
			{
				Title:       "Post with ]]> CDATA killer",
				URL:         "https://example.com/post",
				PublishedDate: now,
				ModifiedDate:  now,
				Keywords:    []string{"tag & stuff", "another<one"},
				Excerpt:     "Text with <tags> & \"quotes\" inside",
				FullContent: htmplt.HTML(`<p>Hello & welcome</p><span>]]>CDATA</span>`),
				SiteTitle:   "Site & \"Title\" <with> specials",
				Options:     FeedOptions{WithDescription: true, WithContent: true},
			},
		},
		LastBuild: now,
	}
	xml, err := eng.RenderRSS(data)
	if err != nil {
		t.Fatalf("RenderRSS(): %v", err)
	}
	out := string(xml)

	// No CDATA blocks should exist
	assertNotContains(t, out, "<![CDATA[")

	// XML special chars should be escaped
	assertContains(t, out, "&lt;p&gt;Hello &amp; welcome&lt;/p&gt;")
	assertContains(t, out, "&lt;span&gt;]]&gt;CDATA&lt;/span&gt;")
	assertContains(t, out, "&lt;tags&gt;")
	assertContains(t, out, "tag &amp; stuff")
	assertContains(t, out, "another&lt;one")

	// ]]> should be escaped (not terminate a CDATA block, since there is none)
	assertContains(t, out, "]]&gt;")

	// Output should be valid XML (basic check: no bare < in content)
	// The ]]> sequence should appear as ]]&gt; in the output
	if strings.Contains(out, "<![CDATA[") {
		t.Error("CDATA blocks should be removed")
	}
}

func TestRenderAtom_XMLEscaping(t *testing.T) {
	eng := mustNewEngine(t)
	now := time.Now()
	data := FeedData{
		Site: SiteConfig{
			Title:   "Atom & \"Feed\" <Title>",
			Slogan:  "Subtitle & more",
			URL:     "https://example.com/",
			Authors: []string{"Author & Co"},
		},
		FeedURL: "https://example.com/atom.xml",
		Items: []FeedItem{
			{
				Title:         "Post with ]]> and <html>",
				URL:           "https://example.com/post",
				PublishedDate: now,
				ModifiedDate:  now,
				Keywords:      []string{"tag & stuff"},
				Excerpt:       "Excerpt with <b>bold</b> & \"quotes\"",
				FullContent:   htmplt.HTML(`<div>Hello &amp; world</div>`),
				SiteTitle:     "Site",
				Options:       FeedOptions{WithDescription: true, WithContent: true},
			},
		},
		LastBuild: now,
	}
	xml, err := eng.RenderAtom(data)
	if err != nil {
		t.Fatalf("RenderAtom(): %v", err)
	}
	out := string(xml)

	// No CDATA blocks
	assertNotContains(t, out, "<![CDATA[")

	// HTML content should be XML-escaped (not raw)
	// &amp; in HTML source becomes &amp;amp; after XML escaping (correct: preserves the entity)
	assertContains(t, out, "&lt;div&gt;Hello &amp;amp; world&lt;/div&gt;")
	assertContains(t, out, "&lt;b&gt;bold&lt;/b&gt;")

	// ]]> should be properly escaped
	assertContains(t, out, "]]&gt;")
}

func TestXmlEscape_NoDoubleEscaping(t *testing.T) {
	// xmlEscape should escape raw content once.
	// text/template does NOT re-escape plain string return values,
	// so there should be no double-escaping.
	tests := []struct {
		name     string
		input    string
		want     string
		notWant  string // must NOT appear (would indicate double-escaping)
	}{
		{
			name:    "ampersand",
			input:   "A & B",
			want:    "&amp;",
			notWant: "&amp;amp;",
		},
		{
			name:    "less-than",
			input:   "a < b",
			want:    "&lt;",
			notWant: "&amp;lt;",
		},
		{
			name:    "greater-than",
			input:   "a > b",
			want:    "&gt;",
			notWant: "&amp;gt;",
		},
		{
			name:  "cdata-killer",
			input: "hello ]]>&gt; world",
			want:  "]]&gt;",
		},
		{
			name:  "raw-html",
			input: "<p>&lt;escaped&gt;</p>",
			want:  "&lt;p&gt;&amp;lt;escaped&amp;gt;&lt;/p&gt;",
			notWant: "&amp;lt;p&gt;",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := xmlEscape(tc.input)
			if !strings.Contains(got, tc.want) {
				t.Errorf("xmlEscape(%q) = %q, want contains %q", tc.input, got, tc.want)
			}
			if tc.notWant != "" && strings.Contains(got, tc.notWant) {
				t.Errorf("xmlEscape(%q) = %q, contains %q (double-escaped?)", tc.input, got, tc.notWant)
			}
		})
	}
}

func assertContains(t *testing.T, s, substr string) {
	if !strings.Contains(s, substr) {
		t.Errorf("expected output to contain %q", substr)
	}
}

func assertNotContains(t *testing.T, s, substr string) {
	if strings.Contains(s, substr) {
		t.Errorf("expected output to NOT contain %q", substr)
	}
}
