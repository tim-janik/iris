// This Source Code Form is licensed MPL-2.0: http://mozilla.org/MPL/2.0
package main

import (
	"html/template"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tim-janik/iris/htmlutil"
	"github.com/tim-janik/iris/templates"
)

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
// computeDirInfo returns metadata for a directory path (e.g. "2007" or "."):
// dirName ("/2007/"), depth (path segments below the root) and root (the
// relative path back to the site root). Shared by computePathInfo (files) and
// computePathInfoForDir (auto-generated dirindex pages).
