package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tim-janik/iris/pageclass"
	"github.com/tim-janik/iris/templates"
)

func TestRenderAllPagesReturnsWriteErrors(t *testing.T) {
	outDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outDir, "blocked"), []byte("file"), 0644); err != nil {
		t.Fatal(err)
	}
	pages := []*InputPage{{RelPath: "blocked/page.md", OutputPath: "blocked/page.html", Type: pageclass.PagePage}}
	if err := renderAllPages(mustTestEngine(t), pages, templates.SiteConfig{}, outDir); err == nil {
		t.Fatal("renderAllPages accepted a blocked destination")
	}
}

func TestGeneratedFilesReturnWriteErrors(t *testing.T) {
	eng := mustTestEngine(t)
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	for _, name := range []string{"rss2.xml", "sitemap.xml"} {
		t.Run(name, func(t *testing.T) {
			outDir := t.TempDir()
			if err := os.Mkdir(filepath.Join(outDir, name), 0755); err != nil {
				t.Fatal(err)
			}
			var err error
			if name == "rss2.xml" {
				_, err = generateFeeds(eng, nil, SiteConfig{URL: "https://example.com"}, templates.SiteConfig{URL: "https://example.com"}, outDir, now)
			} else {
				err = generateSitemap(eng, nil, SiteConfig{URL: "https://example.com"}, outDir, nil, now)
			}
			if err == nil {
				t.Fatalf("accepted blocked %s", name)
			}
		})
	}
}

func TestRecordServeReturnsServerErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "page.md"), []byte("page"), 0644); err != nil {
		t.Fatal(err)
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "failed", http.StatusInternalServerError)
	})
	if err := recordServe(handler, root, t.TempDir()); err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("recordServe error = %v", err)
	}
}
