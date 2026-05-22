// This Source Code Form is licensed MPL-2.0: http://mozilla.org/MPL/2.0
package pageclass

import (
	"testing"

	"github.com/tim-janik/iris/globstar"
)

// TestClassifyPage tests the PageType classification of output paths.
func TestClassifyPage(t *testing.T) {
	tests := []struct {
		path string
		want PageType
	}{
		{"index.html", PageTopIndex},
		{"2024/hello.html", PagePost},
		{"2018/post.html", PagePost},
		{"2018/old/post.html", PagePage}, // nested under year dir → not a post
		{"about.html", PagePage},
		{"docs/guide.html", PagePage},
		{"posts/index.html", PageDirIndex},
		{"2024/index.html", PageDirIndex},
	}

	for _, tt := range tests {
		got := ClassifyPage(tt.path)
		if got != tt.want {
			t.Errorf("ClassifyPage(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

// TestClassifyStatic tests static file classification (PageCopy vs PageAsset).
func TestClassifyStatic(t *testing.T) {
	assetMatcher, _ := globstar.NewMatcher([]string{"assets/**", "*.png", "*.css"})

	tests := []struct {
		path string
		want PageType
	}{
		{"style.html", PageCopy},          // static HTML → copy + sitemap
		{"google0a233ae1380d4c7a.html", PageCopy},
		{"assets/logo.png", PageAsset},    // matches asset glob → copy only
		{"assets/css/main.css", PageAsset},
		{"photo.png", PageAsset},
		{"data.json", PageCopy},           // not in asset glob → copy + sitemap
	}

	for _, tt := range tests {
		got := ClassifyStatic(tt.path, assetMatcher)
		if got != tt.want {
			t.Errorf("ClassifyStatic(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}

	// With no asset matcher, everything is PageCopy
	got := ClassifyStatic("anything.txt", nil)
	if got != PageCopy {
		t.Errorf("ClassifyStatic with nil matcher = %v, want PageCopy", got)
	}
}

// TestPageTypeMethods tests the PageType enum helper methods.
func TestPageTypeMethods(t *testing.T) {
	tests := []struct {
		pt             PageType
		str            string
		isPost         bool
		needsRender    bool
		needsGit       bool
		needsSitemap   bool
	}{
		{PagePost, "post", true, true, true, true},
		{PagePage, "page", false, true, true, true},
		{PageTopIndex, "topindex", false, true, false, true},
		{PageDirIndex, "dirindex", false, true, false, true},
		{PageCopy, "copy", false, false, true, true},
		{PageAsset, "asset", false, false, false, false},
	}

	for _, tt := range tests {
		pt := tt.pt
		if pt.String() != tt.str {
			t.Errorf("PageType(%d).String() = %q, want %q", pt, pt.String(), tt.str)
		}
		if pt.IsPost() != tt.isPost {
			t.Errorf("PageType(%d).IsPost() = %v, want %v", pt, pt.IsPost(), tt.isPost)
		}
		if pt.NeedsRender() != tt.needsRender {
			t.Errorf("PageType(%d).NeedsRender() = %v, want %v", pt, pt.NeedsRender(), tt.needsRender)
		}
		if pt.NeedsGit() != tt.needsGit {
			t.Errorf("PageType(%d).NeedsGit() = %v, want %v", pt, pt.NeedsGit(), tt.needsGit)
		}
		if pt.NeedsSitemap() != tt.needsSitemap {
			t.Errorf("PageType(%d).NeedsSitemap() = %v, want %v", pt, pt.NeedsSitemap(), tt.needsSitemap)
		}
	}
}
