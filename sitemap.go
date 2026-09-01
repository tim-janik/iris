// This Source Code Form is licensed MPL-2.0: http://mozilla.org/MPL/2.0
package main

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/tim-janik/iris/pageclass"
	"github.com/tim-janik/iris/templates"
)

func generateDirIndices(eng *templates.Engine, pages []*InputPage, siteGo templates.SiteConfig, outputDir string, now time.Time) []templates.SitemapEntry {
	var sitemapEntries []templates.SitemapEntry
	dirs := findDirs(pages)
	for _, dir := range dirs {
		// filepath.Join normalizes "./index.html" to "index.html" so the skip
		// check below matches the root page's OutputPath and the auto dirindex
		// never overwrites a rendered root index.
		indexPath := filepath.Join(dir, "index.html")

		// Skip if already rendered as a page
		if slices.ContainsFunc(pages, func(pg *InputPage) bool { return pg.OutputPath == indexPath }) {
			continue
		}

		dirName, depth, root := computePathInfoForDir(dir)

		// Collect posts in this directory. The root dirindex lists all posts;
		// subdirectory dirindices list only posts that live in exactly that
		// directory (not in descendant directories).
		var feedItems []templates.FeedItem
		for _, pg := range pages {
			if !pg.Type.IsPost() {
				continue
			}
			if dir != "." && pg.DirName != dirName {
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
// Mirrors Python Page._special_score(). Callers pass absolute loc URLs built
// from the configured site URL, so comparisons run against the URL path.
func specialScore(loc string) int {
	path := loc
	if u, err := url.Parse(loc); err == nil && u.Path != "" {
		path = u.Path
	}
	if path == "/sitemap.xml" || path == "/index.html" || path == "/index.htm" || path == "/" {
		return +10
	}
	if strings.Contains(path, "/index.htm") {
		return -2
	}
	// Google verification files
	if matched, _ := regexp.MatchString(`/google[0-9a-f]+\.html`, path); matched {
		return -10
	}
	if strings.HasSuffix(path, ".xml") {
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
