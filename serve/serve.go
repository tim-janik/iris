// Package serve provides an HTTP server that renders markdown files to HTML on-the-fly.
//
// It walks the given root directory for .md files, converts them via pandoc,
// and serves the rendered HTML at the configured port.
package serve

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	htmplt "html/template"

	"github.com/tim-janik/iris/adoc"
	"github.com/tim-janik/iris/editlink"
	"github.com/tim-janik/iris/htmlutil"
	"github.com/tim-janik/iris/mimetype"
	"github.com/tim-janik/iris/pandoc"
	"github.com/tim-janik/iris/templates"
	"go.yaml.in/yaml/v4"
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
	// Site holds site-level configuration (title, slogan, stylesheet, etc.).
	Site templates.SiteConfig
}

// serveFrontmatter holds the minimal YAML frontmatter parsed for serve.
type serveFrontmatter struct {
	Title    string   `yaml:"title"`
	Keywords []string `yaml:"keywords"`
}

// parseServeFrontmatter splits content into frontmatter and body.
// Returns empty frontmatter if no YAML block is present.
func parseServeFrontmatter(content []byte) (*serveFrontmatter, string) {
	fm := &serveFrontmatter{}
	text := string(content)
	if !strings.HasPrefix(text, "---\n") {
		return fm, text
	}
	idx := strings.Index(text[4:], "\n---\n")
	if idx == -1 {
		idx = strings.Index(text[4:], "\n...\n")
	}
	if idx == -1 {
		return fm, text
	}
	yaml.Unmarshal([]byte(text[4:4+idx]), fm)
	return fm, strings.TrimLeft(text[4+idx+4:], "\n")
}

// normalizePath ensures the URL path starts with a slash.
func normalizePath(urlPath string) string {
	if !strings.HasPrefix(urlPath, "/") {
		return "/" + urlPath
	}
	return urlPath
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
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		urlPath := normalizePath(r.URL.Path)

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
			w.Header().Set("Content-Type", mimetype.Lookup(ext))
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

		// Parse frontmatter for title
		fm, _ := parseServeFrontmatter(data)

		// Convert via pandoc or asciidoctor
		var htmlStr string
		if strings.HasSuffix(absPath, ".adoc") {
			htmlStr, err = adoc.Convert(s.AdocConfig, data)
		} else {
			htmlStr, err = pandoc.Convert(cfg, data, "")
		}
		if err != nil {
			log.Printf("[500] %s -> %s: %v", urlPath, absPath, err)
			http.Error(w, fmt.Sprintf("Internal Server Error: %v", err), http.StatusInternalServerError)
			return
		}

		// Resolve title: frontmatter > pandoc/asciidoctor extracted <h1>
		title := fm.Title
		if title == "" {
			if doc, pErr := htmlutil.Parse(htmlStr); pErr == nil {
				if h1 := htmlutil.FindByTag(doc, "h1"); h1 != nil {
					title = htmlutil.Text(h1)
				}
			}
		}

		// Render through serve.html template
		serveData := templates.ServeData{
			Site:  s.Site,
			Title: title,
			Content: htmplt.HTML(htmlStr),
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
