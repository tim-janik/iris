// This Source Code Form is licensed MPL-2.0: http://mozilla.org/MPL/2.0
package main

import (
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/tim-janik/iris/adoc"
	"github.com/tim-janik/iris/frontmatter"
	"github.com/tim-janik/iris/globstar"
	"github.com/tim-janik/iris/pageclass"
	"github.com/tim-janik/iris/pandoc"
	"github.com/tim-janik/iris/templates"
	"golang.org/x/sync/errgroup"
)

const dateLayout = "2006-01-02" // Go reference time: YYYY-MM-DD

// ---------------------------------------------------------------------------
// Markdown frontmatter parsing
// ---------------------------------------------------------------------------

// Frontmatter is the shared frontmatter model used by both the SSG and serve.
type Frontmatter = frontmatter.Frontmatter

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

	fm, body := frontmatter.Parse(data, rel)
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
// SSG rendering
// ---------------------------------------------------------------------------

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
