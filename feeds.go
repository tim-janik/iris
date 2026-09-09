// This Source Code Form is licensed MPL-2.0: http://mozilla.org/MPL/2.0
package main

import (
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/tim-janik/iris/htmlutil"
	"github.com/tim-janik/iris/templates"
)

func newFeedItem(pg *InputPage, siteURL string, siteTitle string, descLen int) templates.FeedItem {
	excerpt := truncateExcerpt(stripTags(htmlutil.StripElements(pg.Rendered.Content, "figure")), descLen)
	return templates.FeedItem{
		Title:         pg.Front.Title,
		URL:           templates.JoinURLPath(siteURL, cleanURL(pg.OutputPath)),
		LinkHref:      templates.EncodeURLPath(cleanURL(pg.OutputPath)),
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
func generateFeeds(eng *templates.Engine, pages []*InputPage, site SiteConfig, siteGo templates.SiteConfig, outputDir string, now time.Time) ([]templates.SitemapEntry, error) {
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

	// Use the newest entry change time for feed metadata. Empty feeds fall
	// back to the build time, so generated dates never use the zero time.
	var lastBuild time.Time
	for _, item := range feedItems {
		entryTime := item.ModifiedDate
		if entryTime.IsZero() {
			entryTime = item.PublishedDate
		}
		if entryTime.After(lastBuild) {
			lastBuild = entryTime
		}
	}
	if lastBuild.IsZero() {
		lastBuild = now
	}

	// Helper to track successfully written feed files for sitemap
	feedLastBuild := lastBuild

	// Feed self-links: use the configured feed_url when set, falling back to
	// the conventional feed paths below the site URL.
	rssURL := site.FeedURL
	if rssURL == "" {
		rssURL = templates.JoinURLPath(site.URL, "rss2.xml")
	}
	atomURL := site.FeedURL
	if atomURL == "" {
		atomURL = templates.JoinURLPath(site.URL, "atom.xml")
	}
	// addFeedSitemapEntry records a written feed file for the sitemap. With a
	// custom feed_url both feeds share one URL, which must be listed only once.
	addFeedSitemapEntry := func(loc string) {
		for _, e := range sitemapEntries {
			if e.Loc == loc {
				return
			}
		}
		sitemapEntries = append(sitemapEntries, templates.SitemapEntry{
			Loc:        loc,
			Priority:   "0.6",
			Changefreq: "weekly",
			LastMod:    feedLastBuild.Format(dateLayout),
		})
	}
	// RSS 2.0
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
		return nil, fmt.Errorf("render rss2: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "rss2.xml"), rssXML, 0644); err != nil {
		return nil, fmt.Errorf("write rss2.xml: %w", err)
	}
	log.Printf("  rss2 -> rss2.xml")
	addFeedSitemapEntry(rssURL)

	// Atom
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
		return nil, fmt.Errorf("render atom: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "atom.xml"), atomXML, 0644); err != nil {
		return nil, fmt.Errorf("write atom.xml: %w", err)
	}
	log.Printf("  atom -> atom.xml")
	addFeedSitemapEntry(atomURL)
	return sitemapEntries, nil
}
