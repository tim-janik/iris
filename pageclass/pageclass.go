// Package pageclass classifies files for the SSG pipeline.
package pageclass

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tim-janik/iris/globstar"
)

// ---------------------------------------------------------------------------
// PageType enum
// ---------------------------------------------------------------------------

// PageType identifies how a file should be processed in the SSG pipeline.
type PageType int

const (
	PagePost      PageType = iota // blog post (in a YYYY/ directory), converted + rendered
	PagePage                      // static page, converted + rendered
	PageTopIndex                  // site root index, rendered (no conversion)
	PageDirIndex                  // directory listing index, rendered (no conversion)
	PageCopy                      // static file copied verbatim + sitemap entry (git dates)
	PageAsset                     // static file copied verbatim, no sitemap entry
)

// String returns the human-readable name of the page type.
func (t PageType) String() string {
	switch t {
	case PagePost:
		return "post"
	case PagePage:
		return "page"
	case PageTopIndex:
		return "topindex"
	case PageDirIndex:
		return "dirindex"
	case PageCopy:
		return "copy"
	case PageAsset:
		return "asset"
	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}

// IsPost reports whether this is a blog post page.
func (t PageType) IsPost() bool {
	return t == PagePost
}

// NeedsRender reports whether this page type requires template rendering.
// Post, page, topindex, and dirindex all get rendered through templates.
// Copy and asset are copied verbatim.
func (t PageType) NeedsRender() bool {
	return t <= PageDirIndex
}

// NeedsGit reports whether this page type needs git date lookup.
// Post, page, and copy have source files on disk. Topindex and dirindex
// are auto-generated; asset skips git entirely.
func (t PageType) NeedsGit() bool {
	return t == PagePost || t == PagePage || t == PageCopy
}

// NeedsSitemap reports whether this page type should appear in the sitemap.
// Post, page, and copy get sitemap entries. Asset does not.
func (t PageType) NeedsSitemap() bool {
	return t <= PageCopy
}

// ---------------------------------------------------------------------------
// Classification
// ---------------------------------------------------------------------------

var yearDirRe = regexp.MustCompile(`^(\d{4})/$`)

// ClassifyPage determines the PageType for a rendered output path (.html).
func ClassifyPage(relPath string) PageType {
	if relPath == "index.html" {
		return PageTopIndex
	}
	if strings.HasSuffix(relPath, "/index.html") {
		return PageDirIndex
	}
	// Check if the path starts with a year directory (e.g., 2024/my-post.md)
	if yearDirRe.MatchString(filepath.Dir(relPath)+"/") {
		return PagePost
	}
	return PagePage
}

// ClassifyStatic determines whether a non-conversion file (not .md/.adoc)
// should be copied with a sitemap entry (PageCopy) or copied without
// a sitemap entry (PageAsset). Files matching assetGlob get PageAsset;
// all other included static files get PageCopy.
//
// assetMatcher is the pre-compiled AssetGlob matcher; nil means no
// asset patterns configured, so all static files are PageCopy.
func ClassifyStatic(relPath string, assetMatcher *globstar.Matcher) PageType {
	if assetMatcher != nil && assetMatcher.Match(relPath) {
		return PageAsset
	}
	return PageCopy
}
