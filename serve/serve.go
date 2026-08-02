// Package serve provides an HTTP server that renders markdown files to HTML on-the-fly.
//
// It walks the given root directory for .md files, converts them via pandoc,
// and serves the rendered HTML at the configured port.
package serve

import (
	"encoding/json"
	"fmt"
	htmplt "html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/tim-janik/iris/adoc"
	"github.com/tim-janik/iris/editlink"
	"github.com/tim-janik/iris/frontmatter"
	"github.com/tim-janik/iris/htmlutil"
	"github.com/tim-janik/iris/mimetype"
	"github.com/tim-janik/iris/pandoc"
	"github.com/tim-janik/iris/templates"
)

// Server holds configuration for the markdown HTTP server.
type Server struct {
	// Root is the directory whose markdown/asciidoc files are served.
	Root string
	// Port is the TCP port the server listens on.
	Port int
	// PandocConfig controls pandoc invocation; zero value uses defaults.
	PandocConfig pandoc.Config
	// AdocConfig controls asciidoctor invocation; zero value uses defaults.
	AdocConfig adoc.Config
	// EditLinkCmd is the command template for opening source files in an editor.
	// If empty, edit links are disabled.
	//
	// Supported placeholders:
	//   %s — source file path
	//   %u — line number (substituted only if present in template)
	//
	// Example:
	//   gnome-terminal -- $EDITOR +%u %s
	EditLinkCmd string
	// TemplateDir is a custom template directory (overrides embedded templates).
	TemplateDir string
	// FaviconPath is the path to a favicon file served at /favicon.ico.
	FaviconPath string
	// Site holds site-level configuration (title, slogan, stylesheet, etc.).
	Site templates.SiteConfig
}

func hasMarkdownH1(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "#\t") {
			return true
		}
	}
	return false
}

// extractBodyAndTitle parses a full HTML document and returns the body's inner
// HTML (including any <h1>) and the page title.
// Title resolution mirrors pandoc.extractTitle: <h1> takes priority over <title>.
// Pandoc emits <title>-</title> as a placeholder when no metadata title is set;
// treat that as empty so the caller can fall back to the filename.
func extractBodyAndTitle(htmlStr string) (string, string) {
	doc, err := htmlutil.Parse(htmlStr)
	if err != nil {
		return htmlStr, ""
	}
	body := htmlutil.FindByTag(doc, "body")
	if body == nil {
		return htmlStr, ""
	}
	// <h1> first (matches pandoc.extractTitle), then <title>, then empty
	var title string
	if h1 := htmlutil.FindByTag(body, "h1"); h1 != nil {
		title = htmlutil.Text(h1)
	} else if t := htmlutil.FindByTag(doc, "title"); t != nil {
		title = htmlutil.Text(t)
	}
	if title == "-" {
		title = ""
	}
	return strings.TrimSpace(htmlutil.InnerHTML(body)), title
}

// normalizePath ensures the URL path starts with a slash.
func normalizePath(urlPath string) string {
	if !strings.HasPrefix(urlPath, "/") {
		return "/" + urlPath
	}
	return urlPath
}

// metadataRoute reports whether urlPath names the special per-directory route.
func metadataRoute(urlPath string) bool {
	return urlPath == "/..~meta~" || strings.HasSuffix(urlPath, "/..~meta~")
}

// resolveMetadataDir maps /foo/..~meta~ to a real directory under root. It
// rejects traversal components and symlinks that resolve outside the root.
func resolveMetadataDir(root, urlPath string) (string, error) {
	const suffix = "/..~meta~"
	if !metadataRoute(urlPath) {
		return "", os.ErrNotExist
	}
	dirURL := strings.TrimSuffix(urlPath, suffix)
	if dirURL == "" {
		dirURL = "/"
	}
	for _, component := range strings.Split(strings.Trim(dirURL, "/"), "/") {
		if component == ".." || component == "." || strings.ContainsRune(component, 0) {
			return "", fmt.Errorf("invalid metadata path")
		}
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(rootReal, filepath.FromSlash(strings.TrimPrefix(dirURL, "/")))
	dirReal, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(dirReal)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("metadata path is not a directory")
	}
	rel, err := filepath.Rel(rootReal, dirReal)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("metadata path escapes serve root")
	}
	return dirReal, nil
}

func writeNoCache(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
}

func serveMetadata(w http.ResponseWriter, r *http.Request, root, urlPath string) {
	writeNoCache(w)
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	dir, err := resolveMetadataDir(root, urlPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Not Found", http.StatusNotFound)
		} else {
			http.Error(w, "Bad metadata path", http.StatusBadRequest)
		}
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		http.Error(w, fmt.Sprintf("Internal Server Error: %v", err), http.StatusInternalServerError)
		return
	}

	rootAbs, _ := filepath.Abs(root)
	objects := make([]map[string]any, 0)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, ".") || filepath.Ext(name) != ".md" {
			continue
		}
		// ReadDir's regular-file check intentionally excludes symlinks. This
		// keeps metadata enumeration from following an unexpected file link.
		if !entry.Type().IsRegular() {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(dir, name))
		if readErr != nil {
			continue
		}
		fm, _ := frontmatter.Parse(data, name)
		rel, relErr := filepath.Rel(rootAbs, filepath.Join(dir, name))
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		cleanPath := "/" + strings.TrimSuffix(filepath.ToSlash(rel), filepath.Ext(rel))
		keywords := fm.Keywords
		if keywords == nil {
			keywords = []string{}
		}
		authors := fm.Authors
		if authors == nil {
			authors = []string{}
		}
		record := map[string]any{
			"title":       fm.Title,
			"description": fm.Description,
			"keywords":    keywords,
			"published":   fm.Published,
			"authors":     authors,
			"url":         cleanPath,
		}
		for key, value := range fm.Raw {
			if _, exists := record[key]; !exists {
				record[key] = value
			}
		}
		objects = append(objects, record)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.Method == http.MethodHead {
		return
	}
	if err := json.NewEncoder(w).Encode(objects); err != nil {
		log.Printf("metadata response: %v", err)
	}
}

func handleMetadataRoute(w http.ResponseWriter, r *http.Request, root string) bool {
	if !metadataRoute(r.URL.Path) {
		return false
	}
	writeNoCache(w)
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return true
	}
	query := r.URL.Query()
	if query.Get("cmd") != "get-frontmatter-array" {
		if query.Get("cmd") == "" {
			http.Error(w, "Missing metadata command", http.StatusBadRequest)
		} else {
			http.Error(w, "Unknown metadata command", http.StatusBadRequest)
		}
		return true
	}
	serveMetadata(w, r, root, r.URL.Path)
	return true
}

// Serve starts the HTTP server and blocks until the server exits or errors.
func (s *Server) Serve() error {
	if s.PandocConfig.InputFormat == "" {
		s.PandocConfig = pandoc.DefaultConfig()
	}
	if s.AdocConfig.Attributes == nil {
		s.AdocConfig = adoc.DefaultConfig()
	}

	eng, err := templates.New(s.TemplateDir)
	if err != nil {
		return fmt.Errorf("init templates: %w", err)
	}

	cfg := s.PandocConfig

	mux := http.NewServeMux()

	// Serve favicon if configured (exact path takes priority over catch-all "/").
	if s.FaviconPath != "" {
		mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
				return
			}
			log.Printf("[200] /favicon.ico -> %s", s.FaviconPath)
			w.Header().Set("Content-Type", "image/x-icon")
			http.ServeFile(w, r, s.FaviconPath)
		})
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if metadataRoute(r.URL.Path) {
			handleMetadataRoute(w, r, s.Root)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		urlPath := normalizePath(r.URL.Path)

		// Redirect .md/.adoc URLs to clean URLs (e.g. /foo.md → /foo)
		// unless ?noredirect is set to request the raw source.
		if ext := filepath.Ext(urlPath); ext == ".md" || ext == ".adoc" {
			if !r.URL.Query().Has("noredirect") {
				if _, err := os.Stat(filepath.Join(s.Root, urlPath)); err == nil {
					target := strings.TrimSuffix(urlPath, ext)
					if target == "" {
						target = "/"
					}
					log.Printf("[302] %s -> %s", urlPath, target)
					http.Redirect(w, r, target, http.StatusFound)
					return
				}
			}
		}

		// Try the path as-is first, then append .md, then .adoc.
		// convertToHTML is true only when we found the file by appending an extension
		// (i.e. the user requested /foo/bar and we resolved it to /foo/bar.md).
		var absPath string
		var found, convertToHTML bool
		if info, err := os.Stat(filepath.Join(s.Root, urlPath)); err == nil && !info.IsDir() {
			absPath = filepath.Join(s.Root, urlPath)
			found = true
		} else {
			for _, ext := range []string{".md", ".adoc"} {
				candidate := urlPath + ext
				if _, err := os.Stat(filepath.Join(s.Root, candidate)); err == nil {
					absPath = filepath.Join(s.Root, candidate)
					found = true
					convertToHTML = true
					break
				}
			}
		}

		if !found {
			log.Printf("[404] %s (not found)", urlPath)
			http.Error(w, fmt.Sprintf("Not Found: %s", urlPath), http.StatusNotFound)
			return
		}

		ext := strings.ToLower(filepath.Ext(absPath))

		// If resolved by extension lookup, convert .md/.adoc to HTML.
		// Otherwise treat as a regular file (passthrough or 404).
		if !convertToHTML {
			if !mimetype.IsPassthrough(ext) {
				log.Printf("[404] %s (unsupported type)", urlPath)
				http.Error(w, fmt.Sprintf("Not Found: %s", urlPath), http.StatusNotFound)
				return
			}
			log.Printf("[200] %s -> %s (passthrough)", urlPath, absPath)
			mimeType := mimetype.Lookup(ext)
			if strings.HasPrefix(mimeType, "text/") {
				mimeType += "; charset=utf-8"
			}
			w.Header().Set("Content-Type", mimeType)
			f, err := os.Open(absPath)
			if err != nil {
				log.Printf("[500] %s: open error: %v", absPath, err)
				http.Error(w, fmt.Sprintf("Internal Server Error: %v", err), http.StatusInternalServerError)
				return
			}
			defer f.Close()
			info, err := f.Stat()
			if err != nil {
				log.Printf("[500] %s: stat error: %v", absPath, err)
				http.Error(w, fmt.Sprintf("Internal Server Error: %v", err), http.StatusInternalServerError)
				return
			}
			http.ServeContent(w, r, info.Name(), info.ModTime(), f)
			return
		}

		data, err := os.ReadFile(absPath)
		if err != nil {
			log.Printf("[500] %s: read error: %v", absPath, err)
			http.Error(w, fmt.Sprintf("Internal Server Error: %v", err), http.StatusInternalServerError)
			return
		}

		// Parse the shared frontmatter model. A synthesized title is passed to
		// pandoc only when the markdown body has no H1; otherwise the existing
		// H1 supplies the title and a command-line title would duplicate it.
		fm, body := frontmatter.Parse(data, filepath.Base(absPath))
		if fm.TitleSynthesized && hasMarkdownH1(body) {
			fm.Title = ""
			fm.TitleSynthesized = false
		}

		// Convert via pandoc or asciidoctor, extract full body (including <h1>)
		var bodyContent string
		var convertedTitle string
		if strings.HasSuffix(absPath, ".adoc") {
			htmlStr, convErr := adoc.Convert(s.AdocConfig, data)
			if convErr != nil {
				log.Printf("[500] %s -> %s: %v", urlPath, absPath, convErr)
				http.Error(w, fmt.Sprintf("Internal Server Error: %v", convErr), http.StatusInternalServerError)
				return
			}
			bodyContent, convertedTitle = extractBodyAndTitle(htmlStr)
		} else {
			pandocTitle := ""
			if fm.TitleSynthesized {
				pandocTitle = fm.Title
			}
			htmlStr, convErr := pandoc.ConvertWithTitle(cfg, data, "", pandocTitle)
			if convErr != nil {
				log.Printf("[500] %s -> %s: %v", urlPath, absPath, convErr)
				http.Error(w, fmt.Sprintf("Internal Server Error: %v", convErr), http.StatusInternalServerError)
				return
			}
			bodyContent, convertedTitle = extractBodyAndTitle(htmlStr)
		}

		// Resolve title: frontmatter > h1 from converter > filename (with extension)
		title := fm.Title
		if title == "" {
			title = convertedTitle
		}
		if title == "" {
			title = filepath.Base(absPath)
		}

		// Render through serve.html template
		serveData := templates.ServeData{
			Site:    s.Site,
			Title:   title,
			Content: htmplt.HTML(bodyContent),
		}
		htmlBytes, err := eng.RenderServe(serveData)
		if err != nil {
			log.Printf("[500] %s -> %s: render error: %v", urlPath, absPath, err)
			http.Error(w, fmt.Sprintf("Internal Server Error: %v", err), http.StatusInternalServerError)
			return
		}

		log.Printf("[200] %s -> %s", urlPath, absPath)
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write(htmlBytes)
	})

	handler := http.Handler(mux)
	if s.EditLinkCmd != "" {
		handler = editlink.Handler(editlink.Config{Cmd: s.EditLinkCmd}, mux, s.Root)
	}

	addr := fmt.Sprintf(":%d", s.Port)
	log.Printf("Serve running at http://localhost%s/", addr)
	log.Printf("Root: %s", s.Root)
	return http.ListenAndServe(addr, handler)
}
