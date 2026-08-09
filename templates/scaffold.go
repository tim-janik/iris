// Package templates provides Go template equivalents for the Jinja2 templates
// in _templates/. It demonstrates the data model, custom template functions,
// and the template inheritance pattern used.
//
// Go template inheritance via parse order:
//   1. Parse the base layout first (defines default blocks via {{define}})
//   2. Parse the child template (overrides specific {{define}}d blocks)
//   3. Execute the "layout" template (for HTML pages) or named template (for feeds)
//
// Because Go's {{define}} templates share a flat namespace, each page type
// (page, post, dirindex, topindex) gets its own *htmplt.Template set so
// that sibling overrides don't leak across page types.
//
// All complex logic (sorting, filtering, date formatting) is handled by
// custom template functions or pre-computed in Go structs.
package templates

import (
	"bytes"
	"embed"
	"fmt"
	htmplt "html/template"
	"html"
	"log"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	txtplt "text/template"
	"time"
)

//go:embed layout.html page.html post.html dirindex.html topindex.html rss2.xml atom.xml sitemap.xml serve.html
var templateFS embed.FS

// ---------------------------------------------------------------------------
// Data structures
// ---------------------------------------------------------------------------

// SiteConfig holds site-level configuration, mirroring the Jinja2 `site` object.
type SiteConfig struct {
	Title       string   // site.title
	Slogan      string   // site.slogan
	Description string   // site.description
	URL         string   // site.url
	Authors     []string // site.authors
	Copyright   string   // site.copyright (optional)
	LogoHref    string   // site.logo_href (optional)
	IconHref    string   // site.icon_href (optional)
	FeedURL     string   // site.feed_url (optional)
	FeedAge     int      // site.feed_age (max age in days for RSS/Atom, -1 = unlimited)
	TeaserLen   int      // site.teaser_len (excerpt length for feeds)
	DescLen     int      // site.desc_len (excerpt length for directory listings)
	Stylesheet  string   // custom stylesheet path; empty = use converter defaults
	TitlePrefix string   // prefix prepended to page titles in <title> (e.g. "📜 ")
}

// PageData holds per-page data, mirroring the Jinja2 `page` object.
// Most method calls from Jinja2 (e.g. page.published('rfc')) are replaced
// by pre-computed fields to keep templates simple.
type PageData struct {
	Title         string // page.title
	Content       htmplt.HTML // page.content (rendered HTML body)
	Header        htmplt.HTML // page.header (HTML for header block)
	FooterUpdated string // page.footer_updated
	Keywords      []string // page.keywords
	Authors       []string // page.get_authors()

	DirName string // page.dirname
	Stem    string // page.stem
	Depth   int    // page.depth (0 = top-level)

	IsPost bool // page.enlisted('posts')

	PublishedDate time.Time // underlying time for page.published()
	ModifiedDate  time.Time // underlying time for page.modified()

	// Computed fields (replacing Jinja2 method calls):
	LUID        string // page.get_luid()
	EmailPath   string // precomputed path for mailto comment URL (e.g., "/2005/stem")
	CommentLink htmplt.HTML // precomputed <a> tag for comment link (bypasses URL escaping)
	// StylesheetHref is the stylesheet link href resolved against the page
	// root (see ResolveStylesheet); empty when no stylesheet is configured.
	StylesheetHref string
}

// ResolveStylesheet resolves a site-wide stylesheet path against a page's
// root prefix, mirroring the original Jinja2 templates
// ({{page.root}}/assets/nirvi.css): a relative path like "assets/site.css"
// or "./assets/site.css" becomes "./assets/site.css" on top-level pages and
// "../assets/site.css" on pages one directory deep, so the link works
// regardless of page depth. Absolute URLs (http(s)://, //) and root-relative
// paths ("/assets/site.css") are returned unchanged.
func ResolveStylesheet(stylesheet, root string) string {
	s := strings.TrimSpace(stylesheet)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "//") || strings.HasPrefix(s, "/") {
		return s
	}
	s = strings.TrimPrefix(s, "./")
	root = strings.TrimRight(root, "/")
	if root == "" {
		root = "."
	}
	return root + "/" + s
}

// FeedItem represents a single item in a feed or directory listing.
// This is a flattened view of PageData with extra computed fields.
type FeedItem struct {
	Title            string
	URL              string    // absolute URL
	LinkHref         string    // relative href for directory listings
	PublishedDate time.Time
	ModifiedDate  time.Time
	Keywords      []string
	Excerpt          string    // truncated description
	FullContent      htmplt.HTML // full HTML content
	SiteTitle        string    // site title (for <source> element)
	Options          FeedOptions
}

// FeedOptions controls what content appears in feed items.
type FeedOptions struct {
	WithDescription bool // tmpl.with_description
	WithContent     bool // tmpl.with_content
}

// TemplateData is the top-level data structure passed to HTML page templates.
type TemplateData struct {
	Site           SiteConfig
	Page           PageData
	Root           string            // relative path prefix, e.g. "../.."
	BodyClass      []string          // CSS body classes (mirrors Jinja2 body_classes list)
	Comments       []htmplt.HTML     // pre-rendered comment HTML (for post pages)
	FeedItems      []FeedItem        // pre-filtered & sorted posts (for dirindex)
	TmplOpts       TemplateOpts      // rendering options
	ShowIndexTitle bool              // whether to show the <h2> directory title
}

// TemplateOpts holds per-template rendering flags.
type TemplateOpts struct {
	WithDescription bool
	WithContent     bool
}

// FeedData is the top-level data structure passed to feed templates.
type FeedData struct {
	Site      SiteConfig
	FeedURL   string
	Items     []FeedItem
	LastBuild time.Time
	Options   FeedOptions
}

// SitemapData is the top-level data structure passed to the sitemap template.
type SitemapData struct {
	Pages []SitemapEntry
}

// SitemapEntry represents a single URL entry in the sitemap.
type SitemapEntry struct {
	Loc        string
	Priority   string
	Changefreq string
	LastMod    string // pre-formatted date (e.g. "2024-01-15")
}

// ---------------------------------------------------------------------------
// Template registry
// ---------------------------------------------------------------------------

// pageType identifies which child template to compose with the layout.
const (
	dateLayout = "2006-01-02"        // YYYY-MM-DD
	iso8601    = "2006-01-02T15:04:05Z" // ISO 8601
)

// Engine holds parsed templates and provides rendering methods.
type Engine struct {
	// HTML page templates use html/template for auto-escaping.
	pageTmpl  *htmplt.Template
	postTmpl  *htmplt.Template
	dirTmpl   *htmplt.Template
	topTmpl   *htmplt.Template
	// Serve template - minimal skeleton for iris serve.
	serveTmpl *htmplt.Template
	// XML feed templates use text/template to avoid over-escaping
	// of XML constructs like <?xml, <![CDATA[ etc.
	feedTmpl  *txtplt.Template
	siteTmpl  *txtplt.Template
}

// ServeData is the top-level data structure for serve.html rendering.
type ServeData struct {
	Site      SiteConfig
	Title     string
	Content   htmplt.HTML
	BodyClass string
}

// New creates a new Engine by parsing Go templates.
// If templateDir is non-empty, templates are loaded from that directory
// (overrides embedded templates). Otherwise, embedded templates are used.
// Each page type is parsed into its own template set to isolate block overrides.
//
// HTML page templates use html/template (auto-escapes HTML content).
// XML feed templates use text/template (no over-escaping of XML constructs).
func New(templateDir string) (*Engine, error) {
	// Shared functions for both html and text templates.
	sharedFuncs := htmplt.FuncMap{
		"formatDate":  formatDate,
		"xmlEscape":   xmlEscape,
		"joinStrings": strings.Join,
		"hasBodyClass": hasBodyClass,
		"sliceFirstN": func(s []string, n int) []string{
			if len(s) <= n {
				return s
			}
			return s[:n]
		},
		// trimSlash strips leading and trailing slashes from a string.
		// Replaces Jinja2's page.dirname.strip('/').
		"trimSlash": func(s string) string {
			return strings.Trim(s, "/")
		},
		// trimSuffix removes a trailing suffix from a string.
		"trimSuffix": func(s, suffix string) string {
			return strings.TrimSuffix(s, suffix)
		},
		// urlJoin joins a base URL and a path, ensuring exactly one slash between them.
		// If path is empty, returns base with a trailing slash.
		"urlJoin": func(base, path string) string {
			if path == "" {
				if strings.HasSuffix(base, "/") {
					return base
				}
				return base + "/"
			}
			return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
		},
	}

	// HTML-specific functions (return htmplt.HTML for raw HTML injection).
	htmlFuncs := make(htmplt.FuncMap)
	for k, v := range sharedFuncs {
		htmlFuncs[k] = v
	}
	htmlFuncs["safeHTML"] = func(s string) htmplt.HTML {
		return htmplt.HTML(s)
	}
	htmlFuncs["htmlComment"] = func(s string) htmplt.HTML {
		return htmplt.HTML("<!-- " + s + " -->")
	}

	// text/template func map (no safeHTML needed — no auto-escaping).
	txtFuncs := make(htmplt.FuncMap)
	for k, v := range sharedFuncs {
		txtFuncs[k] = v
	}

	// --- HTML page templates (html/template) ---
	parseHTML := func(names ...string) (*htmplt.Template, error) {
		if templateDir != "" {
			paths := make([]string, 0, len(names))
			for _, n := range names {
				paths = append(paths, filepath.Join(templateDir, n))
			}
			return htmplt.New("").Funcs(htmlFuncs).ParseFiles(paths...)
		}
		return htmplt.New("").Funcs(htmlFuncs).ParseFS(templateFS, names...)
	}

	// --- XML feed templates (text/template) ---
	parseXML := func(names ...string) (*txtplt.Template, error) {
		if templateDir != "" {
			paths := make([]string, 0, len(names))
			for _, n := range names {
				paths = append(paths, filepath.Join(templateDir, n))
			}
			return txtplt.New("").Funcs(txtFuncs).ParseFiles(paths...)
		}
		return txtplt.New("").Funcs(txtFuncs).ParseFS(templateFS, names...)
	}
	var (
		pageTmpl, postTmpl, dirTmpl, topTmpl, serveTmpl *htmplt.Template
		feedTmpl, siteTmpl                              *txtplt.Template
		err                                             error
	)

	pageTmpl, err = parseHTML("layout.html", "page.html")
	if err != nil {
		return nil, fmt.Errorf("parse page templates: %w", err)
	}

	postTmpl, err = parseHTML("layout.html", "post.html")
	if err != nil {
		return nil, fmt.Errorf("parse post templates: %w", err)
	}

	dirTmpl, err = parseHTML("layout.html", "dirindex.html")
	if err != nil {
		return nil, fmt.Errorf("parse dirindex templates: %w", err)
	}

	topTmpl, err = parseHTML("layout.html", "dirindex.html", "topindex.html")
	if err != nil {
		return nil, fmt.Errorf("parse topindex templates: %w", err)
	}

	feedTmpl, err = parseXML("rss2.xml", "atom.xml")
	if err != nil {
		return nil, fmt.Errorf("parse feed templates: %w", err)
	}

	siteTmpl, err = parseXML("sitemap.xml")
	if err != nil {
		return nil, fmt.Errorf("parse sitemap templates: %w", err)
	}

	serveTmpl, err = parseHTML("serve.html")
	if err != nil {
		return nil, fmt.Errorf("parse serve template: %w", err)
	}

	return &Engine{
		pageTmpl: pageTmpl,
		postTmpl: postTmpl,
		dirTmpl:  dirTmpl,
		topTmpl:  topTmpl,
		feedTmpl:  feedTmpl,
		siteTmpl:  siteTmpl,
		serveTmpl: serveTmpl,
	}, nil
}

// ---------------------------------------------------------------------------
// Rendering methods
// ---------------------------------------------------------------------------

// RenderPage renders a regular page using the page.html template.
// Returns the complete HTML document.
func (e *Engine) RenderPage(data TemplateData) ([]byte, error) {
	data.BodyClass = append(data.BodyClass, "page")
	return executeLayout(e.pageTmpl, data)
}

// RenderPost renders a blog post using the post.html template.
// Returns the complete HTML document including comments.
func (e *Engine) RenderPost(data TemplateData) ([]byte, error) {
	data.BodyClass = append(data.BodyClass, "post")
	return executeLayout(e.postTmpl, data)
}

// RenderDirIndex renders a directory index (blog listing) page.
// FeedItems should be pre-filtered to only posts sharing the same directory.
func (e *Engine) RenderDirIndex(data TemplateData) ([]byte, error) {
	if !hasBodyClass(data.BodyClass, "dirindex") {
		data.BodyClass = append(data.BodyClass, "dirindex")
	}
	return executeLayout(e.dirTmpl, data)
}

// RenderTopIndex renders the top-level index page.
// Combines dirindex content with RSS feed meta links.
func (e *Engine) RenderTopIndex(data TemplateData) ([]byte, error) {
	if !hasBodyClass(data.BodyClass, "dirindex") {
		data.BodyClass = append(data.BodyClass, "dirindex")
	}
	return executeLayout(e.topTmpl, data)
}

// RenderRSS renders an RSS 2.0 feed.
func (e *Engine) RenderRSS(data FeedData) ([]byte, error) {
	var buf bytes.Buffer
	err := e.feedTmpl.ExecuteTemplate(&buf, "rss2", data)
	return buf.Bytes(), err
}

// RenderAtom renders an Atom feed.
func (e *Engine) RenderAtom(data FeedData) ([]byte, error) {
	var buf bytes.Buffer
	err := e.feedTmpl.ExecuteTemplate(&buf, "atom", data)
	return buf.Bytes(), err
}

// RenderSitemap renders a sitemap.xml file.
func (e *Engine) RenderSitemap(data SitemapData) ([]byte, error) {
	var buf bytes.Buffer
	err := e.siteTmpl.ExecuteTemplate(&buf, "sitemap", data)
	return buf.Bytes(), err
}

// RenderServe renders a page using the serve.html minimal skeleton.
// Used by iris serve for on-the-fly rendering with proper <title> and optional stylesheet.
func (e *Engine) RenderServe(data ServeData) ([]byte, error) {
	var buf bytes.Buffer
	err := e.serveTmpl.ExecuteTemplate(&buf, "serve", data)
	return buf.Bytes(), err
}

// executeLayout executes the "layout" template with the given data.
func executeLayout(tmpl *htmplt.Template, data TemplateData) ([]byte, error) {
	var buf bytes.Buffer
	err := tmpl.ExecuteTemplate(&buf, "layout", data)
	return buf.Bytes(), err
}

// ---------------------------------------------------------------------------
// Custom template functions
// ---------------------------------------------------------------------------

// xmlEscape escapes XML special characters in the input.
// Accepts string, []byte, or any fmt.Stringer (e.g. htmplt.HTML).
// Returns html.EscapeString output — safe for direct insertion into XML.
// text/template does not re-escape plain string return values,
// so there is no risk of double-escaping.
func xmlEscape(v any) string {
	switch s := v.(type) {
	case string:
		return html.EscapeString(s)
	case []byte:
		return html.EscapeString(string(s))
	case fmt.Stringer:
		return html.EscapeString(s.String())
	default:
		return html.EscapeString(fmt.Sprint(v))
	}
}

// formatDate formats a time.Time value using one of several named formats.
// Mirrors the Python datetime_format() from old/aux/util.py.
// Supported formats:
//   "day"     -> "2024-01-15"
//   "usday"   -> "2024/01/15"  (note: Python uses %Y/%m/%d, not %m/%d/%Y)
//   "rfc822z" -> "Thu, 07 Mar 2024 16:00:00 +0000" (for RSS feeds)
//   "iso8601" -> "2024-03-08T16:00:00Z" (for Atom feeds)
func formatDate(t time.Time, format string) string {
	switch format {
	case "day":
		return t.Local().Format(dateLayout)
	case "usday":
		return t.Local().Format("2006/01/02") // Python: %Y/%m/%d
	case "rfc822z":
		// Python: "%a, %d %b %Y %H:%M:%S +0000" (hardcoded +0000)
		return t.UTC().Format("Mon, 02 Jan 2006 15:04:05 +0000")
	case "iso8601":
		// Python: dt.isoformat() + 'Z'
		return t.UTC().Format(iso8601)
	default:
		return t.UTC().Format(iso8601)
	}
}

// hasBodyClass checks if a body class string is present in the class list.
// Replaces Jinja2's: {% if not 'dirindex' in body_classes %}
func hasBodyClass(classes []string, target string) bool {
	return slices.Contains(classes, target)
}

// ---------------------------------------------------------------------------
// Helper: pre-process pages into FeedItems
// ---------------------------------------------------------------------------

// BuildFeedItems filters pages to only posts, sorts them by published date
// (newest first), and computes all display fields.
// This replaces the Jinja2 logic:
//   list_pages(age=age) | sort(reverse=true, attribute='published_timestamp')
//   pg.enlisted('posts') and page.shares_dirname(pg)
//
// baseDir is the current page's directory (e.g., "/posts/" or "/").
// Posts are filtered to only those sharing the same directory (shares_dirname).
func BuildFeedItems(
	pages []PageData,
	site SiteConfig,
	baseDir string,
	options FeedOptions,
) []FeedItem {
	var items []FeedItem

	// Normalize baseDir: ensure trailing slash for comparison
	baseDirNorm := baseDir
	if len(baseDirNorm) > 0 && baseDirNorm[len(baseDirNorm)-1] != '/' {
		baseDirNorm = baseDirNorm + "/"
	}
	if baseDirNorm == "/" {
		baseDirNorm = "" // root matches everything
	}

	for _, pg := range pages {
		// Filter: only enlisted posts
		if !pg.IsPost {
			continue
		}

		// Filter: shares_dirname — post's dir must start with base dir
		// For root (baseDirNorm == ""), all posts match
		if baseDirNorm != "" && !strings.HasPrefix(pg.DirName+"/", baseDirNorm) {
			continue
		}

		// Compute the post's full href
		href := pg.DirName + "/" + pg.Stem

		// Compute URL (clean URL: strip .html extension)
		urlPath := strings.TrimSuffix(href+".html", ".html")
		if strings.HasSuffix(urlPath, "/index") {
			urlPath = strings.TrimSuffix(urlPath, "/index") + "/"
		}

		item := FeedItem{
			Title:         pg.Title,
			URL:           site.URL + "/" + strings.TrimPrefix(urlPath, "/"),
			PublishedDate: pg.PublishedDate,
			ModifiedDate:  pg.ModifiedDate,
			Keywords:      pg.Keywords,
			Excerpt:       string(pg.Content), // Content is used as excerpt in test/example context
			FullContent:   pg.Content,
			SiteTitle:     site.Title,
			Options:       options,
		}

		// Compute relative link href (mirrors page.link_href(pg))
		item.LinkHref = computeRelativeHref(baseDir, href)

		items = append(items, item)
	}

	// Sort by published date, newest first
	sort.Slice(items, func(i, j int) bool {
		return items[i].PublishedDate.After(items[j].PublishedDate)
	})

	return items
}

// computeRelativeHref computes the relative URL from the current page's
// directory to the target page. Replaces page.link_href(pg).
// Mirrors Python: os.path.relpath(other.href, os.path.dirname(self.href))
func computeRelativeHref(currentDir, targetHref string) string {
	// Compute dirname of current page
	currentBase := currentDir
	if len(currentBase) > 0 && currentBase[len(currentBase)-1] == '/' {
		currentBase = currentBase[:len(currentBase)-1]
	}
	// Handle root directory
	if currentBase == "" {
		currentBase = "/"
	}
	// Use filepath.Rel for relative path computation
	rel, err := filepath.Rel(currentBase, targetHref)
	if err != nil {
		return targetHref
	}
	return rel
}

// ---------------------------------------------------------------------------
// Example usage
// ---------------------------------------------------------------------------

func Example() {
	// 1. Create the template engine (uses embedded templates)
	eng, err := New("")
	if err != nil {
		log.Fatalf("failed to create template engine: %v", err)
	}

	// 2. Set up site configuration
	site := SiteConfig{
		Title:       "Testbit",
		Slogan:      "Bits and Pieces",
		Description: "A collection of projects, thoughts, and code.",
		URL:         "https://testbit.eu/",
		Authors:     []string{"Tim Janik"},
		Copyright:   "Copyright 2024 Tim Janik",
		FeedURL:     "https://testbit.eu/feed.xml",
		IconHref:    "/favicon.ico",
		FeedAge:     365,
		TeaserLen:   500,
		DescLen:     300,
	}

	// 3. Set up a sample page
	now := time.Now()
	page := PageData{
		Title:         "Hello World",
		Content:       `<h1>Hello World</h1><p>This is my first post.</p>`,
		FooterUpdated: "Last updated: " + now.Format(dateLayout),
		Keywords:      []string{"intro", "hello", "world"},
		Authors:       []string{"Tim Janik"},
		DirName:       "posts",
		Stem:          "hello-world",
		Depth:         1,
		IsPost:        true,
		PublishedDate: now.AddDate(0, 0, -30),
		ModifiedDate:  now,
		LUID:          "abc123",
	}

	// 4a. Render a regular page
	pageData := TemplateData{
		Site:      site,
		Page:      page,
		Root:      "..",
		BodyClass: []string{},
	}
	html, err := eng.RenderPage(pageData)
	if err != nil {
		log.Fatalf("render page: %v", err)
	}
	fmt.Fprintf(os.Stdout, "--- Rendered page (%d bytes) ---\n", len(html))

	// 4b. Render a blog post with comments
	postData := TemplateData{
		Site:      site,
		Page:      page,
		Root:      "..",
		BodyClass: []string{},
		Comments:  []htmplt.HTML{`<p>Nice post!</p>`},
	}
	html, err = eng.RenderPost(postData)
	if err != nil {
		log.Fatalf("render post: %v", err)
	}
	fmt.Fprintf(os.Stdout, "--- Rendered post (%d bytes) ---\n", len(html))

	// 4c. Render a directory index (blog listing)
	allPages := []PageData{page} // in practice, load all pages
	feedItems := BuildFeedItems(allPages, site, "", FeedOptions{})
	indexData := TemplateData{
		Site:      site,
		Page:      PageData{DirName: "", Depth: 0},
		Root:      ".",
		BodyClass: []string{},
		FeedItems: feedItems,
	}
	html, err = eng.RenderDirIndex(indexData)
	if err != nil {
		log.Fatalf("render dirindex: %v", err)
	}
	fmt.Fprintf(os.Stdout, "--- Rendered dirindex (%d bytes) ---\n", len(html))

	// 4d. Render RSS feed
	rssData := FeedData{
		Site:      site,
		FeedURL:   "https://testbit.eu/rss2.xml",
		Items:     feedItems,
		LastBuild: now,
		Options: FeedOptions{
			WithDescription: true,
			WithContent:     false,
		},
	}
	xml, err := eng.RenderRSS(rssData)
	if err != nil {
		log.Fatalf("render rss: %v", err)
	}
	fmt.Fprintf(os.Stdout, "--- Rendered RSS (%d bytes) ---\n", len(xml))

	// 4e. Render Atom feed
	atomData := FeedData{
		Site:      site,
		FeedURL:   "https://testbit.eu/atom.xml",
		Items:     feedItems,
		LastBuild: now,
		Options: FeedOptions{
			WithDescription: true,
			WithContent:     true,
		},
	}
	xml, err = eng.RenderAtom(atomData)
	if err != nil {
		log.Fatalf("render atom: %v", err)
	}
	fmt.Fprintf(os.Stdout, "--- Rendered Atom (%d bytes) ---\n", len(xml))

	// 4f. Render sitemap
	sitemapData := SitemapData{
		Pages: []SitemapEntry{
			{
				Loc:        "https://testbit.eu/posts/hello-world",
				Priority:   "0.8",
				Changefreq: "monthly",
				LastMod:    now.Format(dateLayout),
			},
		},
	}
	xml, err = eng.RenderSitemap(sitemapData)
	if err != nil {
		log.Fatalf("render sitemap: %v", err)
	}
	fmt.Fprintf(os.Stdout, "--- Rendered sitemap (%d bytes) ---\n", len(xml))
}
