// This Source Code Form is licensed MPL-2.0: http://mozilla.org/MPL/2.0
package main

import (
	"flag"
	"fmt"
	"golang.org/x/sync/errgroup"
	"html/template"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"runtime/debug"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/tim-janik/iris/adoc"
	"github.com/tim-janik/iris/frontmatter"
	"github.com/tim-janik/iris/globstar"
	"github.com/tim-janik/iris/htmlutil"
	"github.com/tim-janik/iris/pageclass"
	"github.com/tim-janik/iris/pandoc"
	"github.com/tim-janik/iris/serve"
	"github.com/tim-janik/iris/templates"
)

const dateLayout = "2006-01-02" // Go reference time: YYYY-MM-DD

// ---------------------------------------------------------------------------
// Markdown frontmatter parsing
// ---------------------------------------------------------------------------

// Frontmatter is the shared frontmatter model used by both the SSG and serve.
type Frontmatter = frontmatter.Frontmatter

// parseFrontmatter keeps the historical main-package helper while routing all
// callers through the shared parser. The source name is required for title
// synthesis when a document has no explicit title.
func parseFrontmatter(content []byte, sourceName ...string) (*Frontmatter, string) {
	name := ""
	if len(sourceName) > 0 {
		name = sourceName[0]
	}
	return frontmatter.Parse(content, name)
}

// toString converts an any value to string, returning "" for nil.
func toString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// hasBlankKeywords returns true if the keyword slice is empty or contains
// only blank strings.
func hasBlankKeywords(kw []string) bool {
	return !slices.ContainsFunc(kw, func(k string) bool { return strings.TrimSpace(k) != "" })
}

// ---------------------------------------------------------------------------
// Input file processing
// ---------------------------------------------------------------------------

// InputPage represents a single input file to be processed.
type InputPage struct {
	RelPath    string             // relative path from input dir (e.g., "2024/hello.md")
	OutputPath string             // output path (e.g., "2024/hello.html")
	DirName    string             // directory name (e.g., "/2024")
	Stem       string             // filename without extension (e.g., "hello")
	Type       pageclass.PageType // post, page, copy, asset, etc.
	Depth      int                // directory depth
	Root       string             // relative path to root (e.g., "../..")
	Front      *Frontmatter
	Rendered   *pandoc.Result  // pandoc/asciidoctor-converted HTML
	PubDate    time.Time       // publication date (frontmatter > earliest git commit > mtime)
	ModDate    time.Time       // last-updated date (latest git commit > frontmatter > mtime)
	Comments   []template.HTML // pre-rendered comment HTML (for post pages)
}

// PageLUID returns the stable LUID for this page.
func (pg *InputPage) PageLUID() string {
	return computeLUID(pg.DirName + pg.Stem)
}

// walkFiles walks the input directory, applies the pre-compiled globstar.Filter,
// and returns filtered relative paths. Directories that fail the filter
// are pruned via SkipDir.
func walkFiles(inputDir, outputDir string, filter *globstar.Filter) ([]string, error) {
	outputPrefix := filepath.ToSlash(filepath.Clean(outputDir) + "/")
	var files []string

	err := filepath.WalkDir(inputDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip output directory
		if strings.HasPrefix(filepath.ToSlash(filepath.Clean(path))+"/", outputPrefix) {
			return filepath.SkipDir
		}
		rel, _ := filepath.Rel(inputDir, path)
		if rel == "." {
			return nil
		}
		// Apply filter
		if !filter.ShouldInclude(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	return files, err
}

// processAllFiles is the unified parallel queue that handles all file types.
// .md/.adoc files are converted via pandoc/asciidoctor, rendered, and written.
// Static files are copied verbatim (with git dates for pageclass.PageCopy, without for pageclass.PageAsset).
// Returns all InputPage entries (both rendered and copy types) for downstream use.
func processAllFiles(paths []string, inputDir, outputDir string, workers int, assetMatcher *globstar.Matcher, eng *templates.Engine, siteGo templates.SiteConfig) ([]*InputPage, error) {
	// Pre-allocate output slice to preserve order
	pages := make([]*InputPage, len(paths))

	// Process files concurrently, bounded by the worker count. The first
	// error aborts the run; Wait returns it after all workers finish.
	var g errgroup.Group
	g.SetLimit(workers)
	for idx, rel := range paths {
		idx, rel := idx, rel
		g.Go(func() error {
			ext := filepath.Ext(rel)
			absPath := filepath.Join(inputDir, rel)

			// Classify: .md/.adoc → convert+render, everything else → static
			var pg *InputPage
			var err error
			switch ext {
			case ".md", ".adoc":
				pg, err = processSourceFile(rel, absPath, ext, inputDir)
			default:
				pg, err = processStaticFile(rel, absPath, assetMatcher, inputDir, outputDir)
			}
			if err != nil {
				return err
			}
			pages[idx] = pg
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Filter out nil entries (shouldn't happen, but defensive)
	var valid []*InputPage
	for _, pg := range pages {
		if pg != nil {
			valid = append(valid, pg)
		}
	}
	return valid, nil
}

// processSourceFile handles a .md or .adoc file: read, parse frontmatter,
// convert via pandoc/asciidoctor, fetch git dates, and build InputPage.
func processSourceFile(rel, absPath, ext, inputDir string) (*InputPage, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", rel, err)
	}

	fm, body := parseFrontmatter(data, rel)
	if ext == ".adoc" && fm.TitleSynthesized {
		// AsciiDoc: synthesize the title the way asciidoctor extracts it (first
		// heading of any level) instead of the filename stem, so templates and
		// the issue dashboard see the same title asciidoctor would produce,
		// without running asciidoctor. No cmdline title is passed to
		// asciidoctor (it derives the title from the document itself).
		if t := adoc.TitleFromSource(data); t != "" {
			fm.Title = t
		}
	} else if fm.TitleSynthesized && frontmatter.H1Title(body) != "" {
		// The document already supplies its title through an H1. Do not pass
		// the synthesized filename title to pandoc, which would duplicate it.
		fm.Title = ""
		fm.TitleSynthesized = false
	}

	// Convert via pandoc or asciidoctor
	var rendered *pandoc.Result
	if ext == ".adoc" {
		log.Printf("  asciidoctor %s", rel)
		adocRes, convErr := adoc.ConvertAndDisassemble(adoc.DefaultConfig(), data)
		if convErr != nil {
			return nil, fmt.Errorf("asciidoctor %s: %w", rel, convErr)
		}
		rendered = &pandoc.Result{
			Title:    adocRes.Title,
			Content:  adocRes.Content,
			Header:   adocRes.Header,
			Keywords: adocRes.Keywords,
		}
	} else {
		log.Printf("  pandoc %s", rel)
		pandocTitle := ""
		if fm.TitleSynthesized {
			// Pandoc needs a command-line title for title-less documents that
			// begin below H1; an explicit frontmatter title must not be passed
			// because pandoc already reads it from the document metadata.
			pandocTitle = fm.Title
		}
		rendered, err = pandoc.ConvertAndDisassembleWithTitle(pandoc.DefaultConfig(), data, "", pandocTitle)
		if err != nil {
			return nil, fmt.Errorf("pandoc %s: %w", rel, err)
		}
	}

	if rendered == nil || strings.TrimSpace(rendered.Content) == "" {
		return nil, fmt.Errorf("%s: conversion produced empty output", rel)
	}

	// Merge pandoc/asciidoctor-extracted metadata into frontmatter.
	// A converter-extracted title is authoritative over a synthesized one:
	// asciidoctor/pandoc plaintext extraction is exact, while our source-level
	// synthesis (filename stem, AsciiDoc heading scan) is best-effort.
	if rendered.Title != "" && (fm.Title == "" || fm.TitleSynthesized) {
		fm.Title = rendered.Title
	}
	if hasBlankKeywords(fm.Keywords) && len(rendered.Keywords) > 0 {
		fm.Keywords = rendered.Keywords
	}

	// Fetch git dates
	gitFirst, gitLast := gitAuthorDates(absPath)
	if !gitFirst.IsZero() {
		log.Printf("  git %s (first: %s, last: %s)", rel, gitFirst.Format(dateLayout), gitLast.Format(dateLayout))
	}

	// Compute output path
	outPath := strings.TrimSuffix(rel, ext) + ".html"

	dirName, depth, root := computePathInfo(rel)

	// Parse dates: frontmatter > git history > file mtime
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}
	modTime := info.ModTime()

	var explicitPub time.Time
	if fm.Published != "" {
		if t, err := time.Parse(dateLayout, fm.Published); err == nil {
			explicitPub = t.UTC() // normalize to UTC
		}
	}
	// Normalize mtime to UTC
	modTime = modTime.UTC()

	var pubDate, modDate time.Time
	if !gitFirst.IsZero() {
		pubDate = gitFirst
		modDate = gitLast
		if !explicitPub.IsZero() {
			pubDate = explicitPub
			if explicitPub.After(gitLast) {
				modDate = explicitPub
			}
		}
	} else {
		pubDate = modTime
		modDate = modTime
		if !explicitPub.IsZero() {
			pubDate = explicitPub
		}
	}

	return &InputPage{
		RelPath:    rel,
		OutputPath: outPath,
		DirName:    dirName,
		Stem:       strings.TrimSuffix(filepath.Base(rel), ext),
		Type:       pageclass.ClassifyPage(outPath),
		Depth:      depth,
		Root:       root,
		Front:      fm,
		Rendered:   rendered,
		PubDate:    pubDate,
		ModDate:    modDate,
	}, nil
}

// processStaticFile handles a non-conversion file: fetch git dates (if pageclass.PageCopy),
// copy file verbatim to output, and build InputPage.
func processStaticFile(rel, absPath string, assetMatcher *globstar.Matcher, inputDir, outputDir string) (*InputPage, error) {
	pt := pageclass.ClassifyStatic(rel, assetMatcher)

	// Fetch git dates for pageclass.PageCopy (sitemap lastmod); skip for pageclass.PageAsset
	var gitFirst, gitLast time.Time
	if pt.NeedsGit() {
		gitFirst, gitLast = gitAuthorDates(absPath)
		if !gitFirst.IsZero() {
			log.Printf("  git %s (first: %s, last: %s)", rel, gitFirst.Format(dateLayout), gitLast.Format(dateLayout))
		}
	}

	// Copy file verbatim to output
	srcData, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", rel, err)
	}
	dstPath := filepath.Join(outputDir, rel)
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", filepath.Dir(dstPath), err)
	}
	if err := os.WriteFile(dstPath, srcData, 0644); err != nil {
		return nil, fmt.Errorf("write %s: %w", dstPath, err)
	}
	log.Printf("  copy %s", rel)

	// Compute dates for sitemap lastmod
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}
	modTime := info.ModTime().UTC()

	var pubDate, modDate time.Time
	if pt.NeedsGit() && !gitFirst.IsZero() {
		pubDate = gitFirst
		modDate = gitLast
	} else {
		pubDate = modTime
		modDate = modTime
	}

	dirName, depth, root := computePathInfo(rel)

	return &InputPage{
		RelPath:    rel,
		OutputPath: rel, // static files keep their path
		DirName:    dirName,
		Stem:       strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel)),
		Type:       pt,
		Depth:      depth,
		Root:       root,
		Front:      &Frontmatter{Raw: make(map[string]string)},
		PubDate:    pubDate,
		ModDate:    modDate,
	}, nil
}

// gitAuthorDates returns the earliest and latest author dates for the given file path.
// Returns zero times if the file is not tracked by git or git is unavailable.
func gitAuthorDates(filePath string) (first, last time.Time) {
	// Use -C to change to the file's directory, then reference by basename.
	// This avoids git rejecting absolute paths outside its working tree.
	// Fetch ALL commit dates (no -1 limit), sorted newest-first.
	cmd := exec.Command("git", "-C", filepath.Dir(filePath), "log", "--format=%aI", "--", filepath.Base(filePath))
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("  git error %s: %v (%s)", filePath, err, strings.TrimSpace(string(out)))
		return
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return
	}
	// First line is the latest commit (git log default order: newest first)
	last, err = time.Parse(time.RFC3339, lines[0])
	if err != nil {
		log.Printf("  git parse error %s: %q", filePath, lines[0])
		return
	}
	// Last line is the earliest commit
	first, err = time.Parse(time.RFC3339, lines[len(lines)-1])
	if err != nil {
		log.Printf("  git parse error %s: %q", filePath, lines[len(lines)-1])
		return
	}
	// Normalize all times to UTC so direct .Format() call sites (footer
	// "Last updated", sitemap lastmod, feed lastBuildDate) are stable across
	// timezones. Note: formatDate() in templates/scaffold.go re-renders
	// date-only formats in local time (t.Local()) by design — matching the
	// golden output — so page meta dates (DC.date.issued etc.) do depend on
	// the build machine's TZ.
	first = first.UTC()
	last = last.UTC()
	return
}

// ---------------------------------------------------------------------------
// Directory index generation
// ---------------------------------------------------------------------------

// findDirs returns all unique directories that need index.html files.
// Only considers rendered pages (posts/pages), not static assets.
func findDirs(pages []*InputPage) []string {
	dirSet := make(map[string]bool)
	for _, pg := range pages {
		if !pg.Type.NeedsRender() {
			continue
		}
		dir := filepath.Dir(pg.OutputPath)
		if dir != "." {
			dirSet[dir] = true
		}
	}
	// Also add root
	dirSet["."] = true

	dirs := make([]string, 0, len(dirSet))
	for d := range dirSet {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	return dirs
}

// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// SSG rendering
// ---------------------------------------------------------------------------

// SiteConfig from user config or defaults.
type SiteConfig struct {
	Title         string   `toml:"title"          comment:"Site title (used in <title>, feeds, fallback page title)"`
	Slogan        string   `toml:"slogan"         comment:"Short tagline displayed in the header"`
	Description   string   `toml:"description"    comment:"Longer description for <meta> tags and feeds"`
	URL           string   `toml:"url"            comment:"Base URL for absolute links and sitemap"`
	Authors       []string `toml:"authors"        comment:"Default authors (used when page frontmatter has none)"`
	Copyright     string   `toml:"copyright"      comment:"Copyright text for footer"`
	FeedURL       string   `toml:"feed_url"       comment:"URL referenced in feed <link> tags"`
	FeedAge       int      `toml:"feed_age"       comment:"Max post age in days for feeds; -1 = no cutoff"`
	TeaserLen     int      `toml:"teaser_len"     comment:"Excerpt length for RSS/Atom feeds (characters)"`
	DescLen       int      `toml:"desc_len"       comment:"Excerpt length for directory index listings (characters)"`
	IconHref      string   `toml:"icon_href"      comment:"Favicon path"`
	LogoHref      string   `toml:"logo_href"      comment:"Logo image path"`
	CommentsDir   string   `toml:"comments_dir"   comment:"Directory containing .eml comment files"`
	CommentsEmail string   `toml:"comments_email" comment:"Email template for comment posting (%s = page LUID)"`
	IncludeGlob   []string `toml:"include_glob"   comment:"Glob patterns for files to process (e.g. \"20*/**/*.md\", \"**/*.adoc\")"`
	AssetGlob     []string `toml:"asset_glob"     comment:"Glob patterns for static assets (copied verbatim, no sitemap entry)"`
	ExcludeGlob   []string `toml:"exclude_glob"   comment:"Glob patterns for files to skip (default: skip _-prefixed files)"`
	Stylesheet    string   `toml:"stylesheet"     comment:"Custom stylesheet path (e.g. \"assets/site.css\"); empty = use converter defaults"`
	TitlePrefix   string   `toml:"title_prefix"   comment:"Prefix prepended to page titles in <title> (e.g. \"📚 \")"`
}

func defaultSiteConfig() SiteConfig {
	return SiteConfig{
		Title:       "Site",
		Slogan:      "",
		Description: "",
		URL:         "https://example.com",
		Authors:     []string{},
		Copyright:   "",
		FeedURL:     "",
		FeedAge:     -1, // -1 = unlimited (no age cutoff)
		TeaserLen:   300,
		DescLen:     240,
		IconHref:    "/favicon.ico",
		IncludeGlob: []string{},
		AssetGlob:   []string{},
		ExcludeGlob: []string{"_*"}, // default: skip config/template prefixes
		TitlePrefix: "",             // "📜 ",
	}
}

// loadSiteConfig loads site configuration from a TOML file.
// If configFile is empty, returns default configuration without reading any file.
// Fields not present in the file keep their default values.
func loadSiteConfig(inputDir, configFile string) SiteConfig {
	site := defaultSiteConfig()

	if configFile == "" {
		log.Printf("no config file specified, using defaults")
		return site
	}

	_, err := toml.DecodeFile(configFile, &site)
	if err != nil {
		log.Printf("failed to read config %s, using defaults: %v", configFile, err)
		return site
	}
	log.Printf("loaded site config from %s", configFile)
	return site
}

// renderPage renders a single InputPage to HTML using the appropriate template.
func renderPage(eng *templates.Engine, pg *InputPage, site templates.SiteConfig) ([]byte, error) {
	// Build body classes: page-<stem> (type class added by scaffold.Render* methods)
	bodyClass := []string{"page-" + strings.ReplaceAll(pg.Stem, ".", "-")}

	// Build footer updated text
	footerUpdated := toString(pg.Front.Raw["footer_updated"])
	if footerUpdated == "" {
		footerUpdated = "Last updated " + pg.ModDate.UTC().Format("2006-01-02")
	}

	// Use pandoc/asciidoctor-converted HTML for content
	fullContent := pg.Rendered.Content

	// Use page authors, falling back to site authors
	authors := pg.Front.Authors
	if len(authors) == 0 {
		authors = site.Authors
	}

	pageData := templates.TemplateData{
		Site:           site,
		ShowIndexTitle: pg.Type == pageclass.PageDirIndex,
		Page: templates.PageData{
			Title:          pg.Front.Title,
			Header:         template.HTML(pg.Rendered.Header),
			Content:        template.HTML(fullContent),
			FooterUpdated:  footerUpdated,
			Keywords:       pg.Front.Keywords,
			Authors:        authors,
			DirName:        pg.DirName,
			Stem:           pg.Stem,
			Depth:          pg.Depth,
			IsPost:         pg.Type.IsPost(),
			PublishedDate:  pg.PubDate,
			ModifiedDate:   pg.ModDate,
			LUID:           computeLUID(pg.DirName + pg.Stem),
			EmailPath:      strings.TrimSuffix(pg.DirName, "/") + "/" + pg.Stem,
			StylesheetHref: templates.ResolveStylesheet(site.Stylesheet, pg.Root),
			CommentLink: template.HTML(fmt.Sprintf(
				`<a href="mailto:newcomment+%s@testbit.eu?subject=Add%%20comment%%20to%%20%s&body=Add%%20comment%%20to%%20%s:%%0a%%0a"
				   title="Send comment to publish via email, the email address itself is not published"
				   >Post comment via email</a>`,
				computeLUID(pg.DirName+pg.Stem),
				strings.TrimSuffix(pg.DirName, "/")+"/"+pg.Stem,
				strings.TrimSuffix(pg.DirName, "/")+"/"+pg.Stem,
			)),
		},
		Root:      pg.Root,
		BodyClass: bodyClass,
		Comments:  pg.Comments,
	}

	switch pg.Type {
	case pageclass.PagePost:
		return eng.RenderPost(pageData)
	case pageclass.PageTopIndex:
		return eng.RenderTopIndex(pageData)
	case pageclass.PageDirIndex:
		return eng.RenderDirIndex(pageData)
	default:
		return eng.RenderPage(pageData)
	}
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Init subcommand — scaffold a default config file
// ---------------------------------------------------------------------------

const defaultConfigPath = "_siteconfig.toml"

// initConfig writes a default config file to the given path.
// If path is empty, defaults to "_siteconfig.toml" in the current directory.
func initConfig(path string) {
	if path == "" {
		path = defaultConfigPath
	}
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create config: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	cfg := defaultSiteConfig()
	if err := writeConfigTOML(f, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "encode config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Created config file: %s\n", path)
}

// writeConfigTOML serializes a SiteConfig to TOML with per-field comments
// sourced from the struct's "comment" tag.
func writeConfigTOML(w io.Writer, cfg SiteConfig) error {
	t := reflect.TypeOf(cfg)
	v := reflect.ValueOf(cfg)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		comment := field.Tag.Get("comment")
		tomlKey := field.Tag.Get("toml")
		if tomlKey == "" {
			tomlKey = field.Name
		}

		// Write comment
		if comment != "" {
			fmt.Fprintf(w, "# %s\n", comment)
		}

		// Format value
		val := v.Field(i)
		switch val.Kind() {
		case reflect.String:
			fmt.Fprintf(w, "%s = %q\n\n", tomlKey, val.String())
		case reflect.Int:
			fmt.Fprintf(w, "%s = %d\n\n", tomlKey, val.Int())
		case reflect.Slice:
			elems := make([]string, val.Len())
			for j := 0; j < val.Len(); j++ {
				elems[j] = strconv.Quote(val.Index(j).String())
			}
			fmt.Fprintf(w, "%s = [%s]\n\n", tomlKey, strings.Join(elems, ", "))
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Index subcommand — generate index.md lines from a list of .md files
// ---------------------------------------------------------------------------

// indexArgs holds the parsed index command arguments.
type indexArgs struct {
	files []string
}

// parseIndexArgs parses command-line arguments for the index subcommand.
func parseIndexArgs() indexArgs {
	fs := flag.NewFlagSet("index", flag.ExitOnError)
	fs.Parse(os.Args[2:])

	args := fs.Args()
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: %s index <file.md> [file2.md ...]\n", os.Args[0])
		os.Exit(1)
	}
	return indexArgs{files: args}
}

// indexMain reads each .md file, extracts title and description from frontmatter
// (falling back to the first h1 for title), and prints index.md lines to stdout.
func indexMain() {
	args := parseIndexArgs()
	for _, filePath := range args.files {
		data, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %s: %v\n", filePath, err)
			continue
		}
		fm, body := parseFrontmatter(data, filePath)

		title := fm.Title
		if title == "" {
			title = frontmatter.H1Title(body)
		}
		if title == "" {
			title = filepath.Base(strings.TrimSuffix(filePath, filepath.Ext(filePath)))
		}

		// Strip .md/.adoc extension for clean URLs (matches iris serve)
		link := filePath
		if ext := filepath.Ext(filePath); ext == ".md" || ext == ".adoc" {
			link = strings.TrimSuffix(filePath, ext)
		}

		if fm.Description != "" {
			fmt.Printf("- [%s](%s) — %s\n", title, link, fm.Description)
		} else {
			fmt.Printf("- [%s](%s)\n", title, link)
		}
	}
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

// ssgArgs holds the parsed SSG command arguments.
type ssgArgs struct {
	clearOutput bool
	inputDir    string
	outputDir   string
	configFile  string    // explicit config file path (-c flag)
	templateDir string    // custom template directory (-t flag, overrides embedded)
	workers     int       // max concurrent pandoc/asciidoctor workers
	now         time.Time // override for time-dependent output (sitemap priorities/changefreq)
}

// parseSSGArgs parses command-line arguments for the ssg subcommand.
func parseSSGArgs() ssgArgs {
	fs := flag.NewFlagSet("ssg", flag.ExitOnError)
	clearOutput := fs.Bool("C", true, "clean output directory before building (default true)")
	configFile := fs.String("c", "", "path to site config file (TOML)")
	templateDir := fs.String("t", "", "custom template directory (overrides embedded templates)")
	workers := fs.Int("j", 0, "max concurrent pandoc/asciidoctor workers (0 = NumCPU)")
	nowFlag := fs.String("now", "", "override current time for time-dependent output (YYYY-MM-DD or RFC3339; default: IRIS_NOW env or real time)")
	fs.Parse(os.Args[2:])

	args := fs.Args()
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s ssg [flags] <input-dir> <output-dir>\n", os.Args[0])
		os.Exit(1)
	}

	inputDir, _ := filepath.Abs(args[0])
	outputDir, _ := filepath.Abs(args[1])

	w := *workers
	if w <= 0 {
		w = runtime.NumCPU()
	}

	return ssgArgs{clearOutput: *clearOutput, inputDir: inputDir, outputDir: outputDir, configFile: *configFile, templateDir: *templateDir, workers: w, now: parseNow(*nowFlag)}
}

// parseNow resolves the effective "current time" for a build: the -now flag
// wins, then the IRIS_NOW environment variable, then real time. Accepts
// YYYY-MM-DD or RFC3339. The result is normalized to UTC so time-dependent
// output (sitemap priorities/changefreq) is reproducible across machines.
func parseNow(nowFlag string) time.Time {
	value := nowFlag
	if value == "" {
		value = os.Getenv("IRIS_NOW")
	}
	if value == "" {
		return time.Now().UTC()
	}
	if t, err := time.Parse(dateLayout, value); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.UTC()
	}
	log.Fatalf("invalid -now value %q: use YYYY-MM-DD or RFC3339", value)
	return time.Time{}
}

// parseInitArgs parses command-line arguments for the init subcommand.
func parseInitArgs() string {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	path := fs.String("o", "", "output file (default: _siteconfig.toml; overwrites if exists)")
	fs.Parse(os.Args[2:])
	return *path
}

// ---------------------------------------------------------------------------
// Serve subcommand — HTTP server for on-the-fly markdown rendering
// ---------------------------------------------------------------------------

// serveArgs holds the parsed serve command arguments.
type serveArgs struct {
	root        string // directory containing markdown files
	port        int    // TCP port to listen on
	editLinkCmd string // command template for edit links (empty = disabled)
	templateDir string // custom template directory (overrides embedded templates)
	faviconPath string // path to favicon file served at /favicon.ico
}

// parseServeArgs parses command-line arguments for the serve subcommand.
func parseServeArgs() serveArgs {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	port := fs.Int("port", 9454, "TCP port to listen on (default: 9454)")
	editLinkCmd := fs.String("editlink", "", "command template to open source file in editor (empty = disabled); use %s for file path, %u for line number")
	templateDir := fs.String("t", "", "custom template directory (overrides embedded templates)")
	faviconPath := fs.String("favicon", "", "path to favicon file served at /favicon.ico")
	fs.Parse(os.Args[2:])

	args := fs.Args()
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s serve [flags] <path>\n", os.Args[0])
		os.Exit(1)
	}

	root, _ := filepath.Abs(args[0])
	return serveArgs{root: root, port: *port, editLinkCmd: *editLinkCmd, templateDir: *templateDir, faviconPath: *faviconPath}
}

// serveMain is the main entry point for the serve subcommand.
func serveMain() {
	args := parseServeArgs()

	if _, err := os.Stat(args.root); os.IsNotExist(err) {
		log.Fatalf("root path does not exist: %s", args.root)
	}

	// Auto-detect config file in root directory; falls back to defaults if missing.
	configFile := ""
	if _, err := os.Stat(filepath.Join(args.root, defaultConfigPath)); err == nil {
		configFile = filepath.Join(args.root, defaultConfigPath)
	}
	site := loadSiteConfig(args.root, configFile)

	srv := &serve.Server{
		Root:         args.root,
		Port:         args.port,
		PandocConfig: pandoc.DefaultConfig(),
		AdocConfig:   adoc.DefaultConfig(),
		EditLinkCmd:  args.editLinkCmd,
		TemplateDir:  args.templateDir,
		FaviconPath:  args.faviconPath,
		Site:         toTemplateSite(site),
	}

	if err := srv.Serve(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// ---------------------------------------------------------------------------

// prepareOutputDir removes and recreates the output directory if requested.
func prepareOutputDir(dir string, clear bool) {
	if clear {
		log.Printf("Cleaning output directory: %s", dir)
		if err := os.RemoveAll(dir); err != nil {
			log.Fatalf("remove output dir: %v", err)
		}
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatalf("create output dir: %v", err)
	}
}

// initEngine creates the template engine.
// If templateDir is non-empty, templates are loaded from that directory.
// Otherwise, embedded templates are used.
func initEngine(templateDir string) *templates.Engine {
	eng, err := templates.New(templateDir)
	if err != nil {
		log.Fatalf("init templates: %v", err)
	}
	return eng
}

// toTemplateSite converts a SiteConfig to templates.SiteConfig.
func toTemplateSite(site SiteConfig) templates.SiteConfig {
	return templates.SiteConfig{
		Title:       site.Title,
		Slogan:      site.Slogan,
		Description: site.Description,
		URL:         site.URL,

		Authors:     site.Authors,
		Copyright:   site.Copyright,
		FeedURL:     site.FeedURL,
		FeedAge:     site.FeedAge,
		TeaserLen:   site.TeaserLen,
		DescLen:     site.DescLen,
		IconHref:    site.IconHref,
		LogoHref:    site.LogoHref,
		Stylesheet:  site.Stylesheet,
		TitlePrefix: site.TitlePrefix,
	}
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "init":
		initConfig(parseInitArgs())
	case "index":
		indexMain()
	case "ssg":
		ssgMain()
	case "serve":
		serveMain()
	case "version":
		printVersion()
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

// printUsage prints the usage summary to stderr.
func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: iris <subcommand> [flags]

Subcommands:
  init      Create _siteconfig.toml with default settings
  index     Generate index.md lines from a list of .md files
  ssg       Build the site (convert, render, generate feeds/sitemap)
  serve     Start an HTTP server that renders .md files to HTML on-the-fly
  version   Print version and build information

Run "iris <subcommand> -h" for more information on a subcommand.

Examples:
  iris index page1.md page2.md
	Print markdown link lines suitable for index.md
  iris serve ./docs --port 9454
	Serves all .md files under ./docs as rendered HTML on localhost:9454
`)
}

// printVersion prints version and VCS build information.
func printVersion() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		fmt.Println("iris (no build info)")
		return
	}
	fmt.Printf("iris %s\n", info.Main.Version)
	if info.Main.Sum != "" {
		fmt.Printf("  module: %s %s\n", info.Main.Path, info.Main.Sum)
	}
	for _, dep := range info.Deps {
		fmt.Printf("  %s %s\n", dep.Path, dep.Version)
	}
	if settings := info.Settings; len(settings) > 0 {
		fmt.Println()
		for _, s := range settings {
			fmt.Printf("  %s=%s\n", s.Key, s.Value)
		}
	}
}

// ssgMain is the main entry point for the ssg subcommand.
func ssgMain() {
	args := parseSSGArgs()
	log.Printf("Input:  %s", args.inputDir)
	log.Printf("Output: %s", args.outputDir)

	prepareOutputDir(args.outputDir, args.clearOutput)

	eng := initEngine(args.templateDir)
	site := loadSiteConfig(args.inputDir, args.configFile)
	// Default feed_url to site URL + "/rss2.xml" when not specified
	if site.FeedURL == "" {
		site.FeedURL = site.URL + "/rss2.xml"
	}
	siteGo := toTemplateSite(site)

	// Candidate files = union(include_glob, asset_glob); files matching neither are skipped
	allInclude := append(append([]string{}, site.IncludeGlob...), site.AssetGlob...)
	fileFilter, err := globstar.NewFilter(allInclude, site.ExcludeGlob)
	if err != nil {
		log.Fatalf("compile file filter: %v", err)
	}

	// Compile asset matcher (copy-only, no sitemap entry)
	assetMatcher, err := globstar.NewMatcher(site.AssetGlob)
	if err != nil {
		log.Fatalf("compile asset matcher: %v", err)
	}

	// Walk + filter
	allFiles, err := walkFiles(args.inputDir, args.outputDir, fileFilter)
	if err != nil {
		log.Fatalf("walk files: %v", err)
	}

	// Sort so classification order is stable regardless of FS walk order
	sort.Strings(allFiles)

	// Unified parallel queue: process all files (convert+render for .md/.adoc,
	// copy for static files, git dates for pageclass.PageCopy)
	pages, err := processAllFiles(allFiles, args.inputDir, args.outputDir, args.workers, assetMatcher, eng, siteGo)
	if err != nil {
		log.Fatalf("process files: %v", err)
	}
	log.Printf("Found %d pages", len(pages))

	// Load comments from .eml files
	loadCommentsForPages(MailboxConfig{
		CommentsDir:   site.CommentsDir,
		CommentsEmail: site.CommentsEmail,
	}, pages)

	// Render pages (only types that need template rendering)
	renderAllPages(eng, pages, siteGo, args.outputDir)

	// Generate directory indices (returns sitemap entries for each dirindex)
	dirIndexEntries := generateDirIndices(eng, pages, siteGo, args.outputDir, args.now)

	// Generate RSS and Atom feeds (returns sitemap entries for feed files)
	feedEntries := generateFeeds(eng, pages, site, siteGo, args.outputDir, args.now)

	// Generate sitemap (after dirindices and feeds so all entries are known)
	allExtra := append(dirIndexEntries, feedEntries...)
	generateSitemap(eng, pages, site, args.outputDir, allExtra, args.now)

	log.Printf("Done. Output in %s", args.outputDir)
}

// loadCommentsForPages loads comments from .eml files and attaches them
// to the corresponding InputPage structs.
func loadCommentsForPages(cfg MailboxConfig, pages []*InputPage) {
	if cfg.CommentsDir == "" {
		return
	}

	log.Printf("Loading comments from %s", cfg.CommentsDir)
	commentsByLuid := LoadComments(cfg, pages)
	if commentsByLuid == nil {
		return
	}

	for _, pg := range pages {
		luid := pg.PageLUID()
		if cmts, ok := commentsByLuid[luid]; ok {
			log.Printf("  %s: %d comment(s)", pg.RelPath, len(cmts))
			htmlCmts := make([]template.HTML, len(cmts))
			for i, c := range cmts {
				htmlCmts[i] = template.HTML(c)
			}
			pg.Comments = htmlCmts
		}
	}
}

// renderAllPages renders each input page and writes the HTML output.
func renderAllPages(eng *templates.Engine, pages []*InputPage, siteGo templates.SiteConfig, outputDir string) {
	for _, pg := range pages {
		// Skip static files (pageclass.PageCopy, pageclass.PageAsset) — they are already copied
		if !pg.Type.NeedsRender() {
			continue
		}
		outPath := filepath.Join(outputDir, pg.OutputPath)
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			log.Printf("  mkdir %s: %v", filepath.Dir(outPath), err)
			continue
		}

		html, err := renderPage(eng, pg, siteGo)
		if err != nil {
			log.Printf("  render %s: %v", pg.RelPath, err)
			continue
		}

		if err := os.WriteFile(outPath, html, 0644); err != nil {
			log.Printf("  write %s: %v", outPath, err)
			continue
		}
		log.Printf("  %s -> %s", pg.RelPath, pg.OutputPath)
	}
}

// generateDirIndices creates index.html files for each directory containing posts.
// Returns sitemap entries for each dirindex page created.
func generateDirIndices(eng *templates.Engine, pages []*InputPage, siteGo templates.SiteConfig, outputDir string, now time.Time) []templates.SitemapEntry {
	var sitemapEntries []templates.SitemapEntry
	dirs := findDirs(pages)
	for _, dir := range dirs {
		indexPath := dir + "/index.html"

		// Skip if already rendered as a page
		if slices.ContainsFunc(pages, func(pg *InputPage) bool { return pg.OutputPath == indexPath }) {
			continue
		}

		dirName, depth, root := computePathInfoForDir(dir)

		// Collect posts in this directory
		var feedItems []templates.FeedItem
		for _, pg := range pages {
			if !pg.Type.IsPost() || !strings.HasPrefix(pg.DirName, dirName) {
				continue
			}
			// D3: compute LinkHref relative to this dirindex directory
			linkHref := cleanURL(pg.OutputPath)
			if dir != "." {
				linkHref = strings.TrimPrefix(linkHref, dir+"/")
			}
			fi := newFeedItem(pg, siteGo.URL, siteGo.Title, siteGo.DescLen)
			fi.LinkHref = linkHref
			feedItems = append(feedItems, fi)
		}

		// Sort by published date (newest first)
		sort.Slice(feedItems, func(i, j int) bool {
			return feedItems[i].PublishedDate.After(feedItems[j].PublishedDate)
		})

		bodyClass := []string{"page-siteindex_", "dirindex"}
		if dir != "." {
			bodyClass = []string{"page-" + strings.ReplaceAll(dir, "/", "-"), "dirindex"}
		}

		pageData := templates.TemplateData{
			Site: siteGo,
			Page: templates.PageData{
				Title:          "", // D2: leave empty so template falls back to site title
				DirName:        dirName,
				FooterUpdated:  "",
				StylesheetHref: templates.ResolveStylesheet(siteGo.Stylesheet, root),
			},
			Root:           root,
			BodyClass:      bodyClass,
			FeedItems:      feedItems,
			ShowIndexTitle: dir != ".",
		}

		var html []byte
		var renderErr error
		if dir == "." {
			html, renderErr = eng.RenderTopIndex(pageData)
		} else {
			html, renderErr = eng.RenderDirIndex(pageData)
		}
		if renderErr != nil {
			log.Printf("  render dirindex %s: %v", indexPath, renderErr)
			continue
		}

		outPath := filepath.Join(outputDir, indexPath)
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			log.Printf("  mkdir %s: %v", filepath.Dir(outPath), err)
			continue
		}
		if err := os.WriteFile(outPath, html, 0644); err != nil {
			log.Printf("  write %s: %v", outPath, err)
			continue
		}
		log.Printf("  dirindex -> %s", indexPath)

		// Collect sitemap entry for this dirindex
		// Use the newest post's modification date in this directory as the dirindex's mod date
		var dirModDate time.Time
		for _, fi := range feedItems {
			if fi.ModifiedDate.After(dirModDate) {
				dirModDate = fi.ModifiedDate
			}
		}
		if dirModDate.IsZero() {
			dirModDate = now
		}
		loc := siteGo.URL + "/" + strings.TrimPrefix(strings.TrimPrefix(cleanURL(indexPath), "/"), "./")
		sitemapEntries = append(sitemapEntries, templates.SitemapEntry{
			Loc:        loc,
			Priority:   calcPriorityForPath(loc, depth),
			Changefreq: calcChangefreqForDate(dirModDate, loc, now),
			LastMod:    dirModDate.Format(dateLayout),
		})
	}
	return sitemapEntries
}

// generateSitemap creates sitemap.xml for all pages.
// extraEntries are additional sitemap entries (e.g. dirindex, feeds) not derived from InputPage.
func generateSitemap(eng *templates.Engine, pages []*InputPage, site SiteConfig, outputDir string, extraEntries []templates.SitemapEntry, now time.Time) {
	var entries []templates.SitemapEntry
	for _, pg := range pages {
		// Only include pages that need sitemap entries AND are web files (HTML)
		// Static assets (images, etc.) are copied but not listed in sitemap
		if !pg.Type.NeedsSitemap() {
			continue
		}
		if pg.Type == pageclass.PageCopy && !strings.HasSuffix(pg.OutputPath, ".html") {
			continue
		}
		// Static HTML files keep .html in URL; rendered pages get clean URLs
		var urlPath string
		if pg.Type == pageclass.PageCopy {
			urlPath = pg.OutputPath
		} else {
			urlPath = cleanURL(pg.OutputPath)
		}
		loc := site.URL + "/" + strings.TrimPrefix(urlPath, "/")
		entries = append(entries, templates.SitemapEntry{
			Loc:        loc,
			Priority:   calcPriority(pg, loc, now),
			Changefreq: calcChangefreq(pg, loc, now),
			LastMod:    pg.ModDate.Format(dateLayout),
		})
	}
	// Merge extra entries (dirindex, feeds)
	entries = append(entries, extraEntries...)

	xml, err := eng.RenderSitemap(templates.SitemapData{Pages: entries})
	if err != nil {
		log.Printf("render sitemap: %v", err)
		return
	}
	if err := os.WriteFile(filepath.Join(outputDir, "sitemap.xml"), xml, 0644); err != nil {
		log.Printf("write sitemap.xml: %v", err)
	} else {
		log.Printf("  sitemap -> sitemap.xml")
	}
}

// calcChangefreq determines sitemap change frequency based on modification age.
// Mirrors Python Page.get_changefreq(): age in days → hourly/daily/weekly/monthly/yearly.
func calcChangefreq(pg *InputPage, loc string, now time.Time) string {
	return calcChangefreqForDate(pg.ModDate, loc, now)
}

// calcChangefreqForDate is the same as calcChangefreq but takes a time directly
// instead of an InputPage. Used for dirindex and feed entries.
func calcChangefreqForDate(modDate time.Time, loc string, now time.Time) string {
	// Special pages get 'always'
	if specialScore(loc) >= 7 {
		return "always"
	}
	// Use modification date for age calculation
	modAge := daysSince(modDate, now)
	if modAge <= 1 {
		return "hourly"
	}
	if modAge <= 7 {
		return "daily"
	}
	if modAge <= 53 {
		return "weekly"
	}
	if modAge <= 397 {
		return "monthly"
	}
	// XML feeds get weekly as a best guess
	if strings.HasSuffix(loc, ".xml") {
		return "weekly"
	}
	return "yearly"
}

// specialScore rates a URL between -10 and +10 for priority calculation.
// Mirrors Python Page._special_score().
func specialScore(loc string) int {
	if loc == "/sitemap.xml" || loc == "/index.html" || loc == "/index.htm" || loc == "/" {
		return +10
	}
	if strings.Contains(loc, "/index.htm") {
		return -2
	}
	// Google verification files
	if matched, _ := regexp.MatchString(`/google[0-9a-f]+\.html`, loc); matched {
		return -10
	}
	if strings.HasSuffix(loc, ".xml") {
		return -3
	}
	return 0
}

// calcPriority computes sitemap priority 0.0-1.0 based on page properties.
// Mirrors Python Page.get_priority(): base 5 + special score + credits, / 10.
func calcPriority(pg *InputPage, loc string, now time.Time) string {
	score := specialScore(loc)
	credits := 0
	// Root level credit
	if pg.Depth == 0 {
		credits++
	}
	// Has input file (not auto-generated)
	if pg.RelPath != "" {
		credits++
	}
	// Is a post
	if pg.Type.IsPost() {
		credits++
	}
	// Published within last year
	pubAge := daysSince(pg.PubDate, now)
	if pubAge <= 366 {
		credits++
	}
	// Modified within last 66 days
	modAge := daysSince(pg.ModDate, now)
	if modAge <= 66 {
		credits++
	}
	return formatPriority(5 + score + credits)
}

// calcPriorityForPath computes priority for a path that isn't an InputPage
// (e.g. dirindex, feeds). Uses depth and a default score.
func calcPriorityForPath(loc string, depth int) string {
	score := specialScore(loc)
	credits := 0
	// Root level credit
	if depth == 0 {
		credits++
	}
	return formatPriority(5 + score + credits)
}

// formatPriority clamps and formats a raw priority score.
func formatPriority(raw int) string {
	prio := float64(raw) / 10.0
	if prio < 0 {
		prio = 0
	}
	if prio > 1.0 {
		prio = 1.0
	}
	return fmt.Sprintf("%.1f", prio)
}

// daysSince returns the number of days between t and now.
func daysSince(t, now time.Time) int {
	return int(now.Sub(t).Hours() / 24)
}

// newFeedItem creates a FeedItem from an InputPage.
func newFeedItem(pg *InputPage, siteURL string, siteTitle string, descLen int) templates.FeedItem {
	excerpt := truncateExcerpt(stripTags(htmlutil.StripElements(pg.Rendered.Content, "figure")), descLen)
	return templates.FeedItem{
		Title:         pg.Front.Title,
		URL:           siteURL + "/" + strings.TrimPrefix(cleanURL(pg.OutputPath), "/"),
		LinkHref:      cleanURL(pg.OutputPath),
		PublishedDate: pg.PubDate,
		ModifiedDate:  pg.ModDate,
		Keywords:      pg.Front.Keywords,
		Excerpt:       excerpt,
		FullContent:   template.HTML(pg.Rendered.Content),
		SiteTitle:     siteTitle,
		Options:       templates.FeedOptions{WithDescription: true, WithContent: false},
	}
}

// generateFeeds creates RSS 2.0 and Atom feeds for all posts.
// Returns sitemap entries for the feed files created.
func generateFeeds(eng *templates.Engine, pages []*InputPage, site SiteConfig, siteGo templates.SiteConfig, outputDir string, now time.Time) []templates.SitemapEntry {
	var sitemapEntries []templates.SitemapEntry
	// Collect all posts sorted by published date (newest first)
	var feedItems []templates.FeedItem
	cutoffAge := time.Duration(siteGo.FeedAge) * 24 * time.Hour
	for _, pg := range pages {
		if !pg.Type.IsPost() {
			continue
		}
		// Feed age cutoff: skip posts older than FeedAge days (-1 = unlimited)
		if siteGo.FeedAge >= 0 {
			age := now.Sub(pg.PubDate)
			if age > cutoffAge {
				continue
			}
		}
		feedItems = append(feedItems, newFeedItem(pg, site.URL, site.Title, site.TeaserLen))
	}
	sort.Slice(feedItems, func(i, j int) bool {
		return feedItems[i].PublishedDate.After(feedItems[j].PublishedDate)
	})

	// Use newest page's published date for reproducible builds
	var lastBuild time.Time
	if len(feedItems) > 0 {
		lastBuild = feedItems[0].PublishedDate
	} // zero time when no posts

	// Helper to track successfully written feed files for sitemap
	feedLastBuild := lastBuild

	// RSS 2.0
	rssURL := site.URL + "/rss2.xml"
	rssData := templates.FeedData{
		Site:      siteGo,
		FeedURL:   rssURL,
		Items:     feedItems,
		LastBuild: lastBuild,
		Options: templates.FeedOptions{
			WithDescription: true,
			WithContent:     false,
		},
	}
	rssXML, err := eng.RenderRSS(rssData)
	if err != nil {
		log.Printf("render rss2: %v", err)
	} else {
		if err := os.WriteFile(filepath.Join(outputDir, "rss2.xml"), rssXML, 0644); err != nil {
			log.Printf("write rss2.xml: %v", err)
		} else {
			log.Printf("  rss2 -> rss2.xml")
			sitemapEntries = append(sitemapEntries, templates.SitemapEntry{
				Loc:        rssURL,
				Priority:   "0.6",
				Changefreq: "weekly",
				LastMod:    feedLastBuild.Format(dateLayout),
			})
		}
	}

	// Atom
	atomURL := site.URL + "/atom.xml"
	atomData := templates.FeedData{
		Site:      siteGo,
		FeedURL:   atomURL,
		Items:     feedItems,
		LastBuild: lastBuild,
		Options: templates.FeedOptions{
			WithDescription: true,
			WithContent:     true,
		},
	}
	atomXML, err := eng.RenderAtom(atomData)
	if err != nil {
		log.Printf("render atom: %v", err)
	} else {
		if err := os.WriteFile(filepath.Join(outputDir, "atom.xml"), atomXML, 0644); err != nil {
			log.Printf("write atom.xml: %v", err)
		} else {
			log.Printf("  atom -> atom.xml")
			sitemapEntries = append(sitemapEntries, templates.SitemapEntry{
				Loc:        atomURL,
				Priority:   "0.6",
				Changefreq: "weekly",
				LastMod:    feedLastBuild.Format(dateLayout),
			})
		}
	}
	return sitemapEntries
}

// computePathInfo computes dirName, depth, and root for a relative file path.
// Used for rendered pages (.md/.adoc) and static copies.
func computePathInfo(rel string) (dirName string, depth int, root string) {
	dir := filepath.Dir(rel)
	dirName = "/" + dir
	if dirName != "/" && !strings.HasSuffix(dirName, "/") {
		dirName += "/"
	}
	if dir == "." {
		return dirName, 1, "."
	}
	levels := strings.Count(filepath.ToSlash(dir), "/") + 1
	return dirName, levels + 1, strings.TrimRight(strings.Repeat("../", levels), "/")
}

// computePathInfoForDir is like computePathInfo but takes a directory path
// directly (not a file path). Used for auto-generated dirindex pages.
// Depth starts at 0 for root and root uses ".." (no trailing slash).
func computePathInfoForDir(dir string) (dirName string, depth int, root string) {
	dirName = "/"
	if dir != "." {
		dirName = "/" + dir
	}
	if dirName != "/" && !strings.HasSuffix(dirName, "/") {
		dirName += "/"
	}
	if dir == "." {
		return dirName, 0, "."
	}
	depth = strings.Count(dir, "/") + 1
	return dirName, depth, strings.TrimRight(strings.Repeat("../", depth), "/")
}

// cleanURL strips the .html extension for clean URLs.
// index.html is converted to trailing slash (e.g. "2005/index.html" → "2005/").
func cleanURL(path string) string {
	if strings.HasSuffix(path, "/index.html") || path == "index.html" {
		dir := strings.TrimSuffix(path, "index.html")
		if dir == "" {
			return "/"
		}
		return dir
	}
	return strings.TrimSuffix(path, ".html")
}
