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

	"github.com/tim-janik/iris/pandoc"
)

// Server holds configuration for the markdown HTTP server.
type Server struct {
	// Root is the directory whose markdown files are served.
	Root string
	// Port is the TCP port the server listens on.
	Port int
	// PandocConfig controls pandoc invocation; zero value uses defaults.
	PandocConfig pandoc.Config
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

	cfg := s.PandocConfig

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		urlPath := normalizePath(r.URL.Path)

		// If path ends with .md, redirect to extensionless URL
		if strings.HasSuffix(urlPath, ".md") {
			cleanPath := strings.TrimSuffix(urlPath, ".md")
			mdAbsPath := filepath.Join(s.Root, urlPath)
			if _, err := os.Stat(mdAbsPath); err == nil {
				log.Printf("[301] %s -> %s", urlPath, cleanPath)
				http.Redirect(w, r, cleanPath, http.StatusMovedPermanently)
				return
			}
		}

		// Try the path as-is first, then append .md
		var absPath string
		var found bool
		if info, err := os.Stat(filepath.Join(s.Root, urlPath)); err == nil && !info.IsDir() {
			absPath = filepath.Join(s.Root, urlPath)
			found = true
		} else if !strings.HasSuffix(urlPath, ".md") {
			mdPath := urlPath + ".md"
			if _, err := os.Stat(filepath.Join(s.Root, mdPath)); err == nil {
				absPath = filepath.Join(s.Root, mdPath)
				found = true
			}
		}

		if !found {
			log.Printf("[404] %s (not found)", urlPath)
			http.Error(w, fmt.Sprintf("Not Found: %s", urlPath), http.StatusNotFound)
			return
		}

		data, err := os.ReadFile(absPath)
		if err != nil {
			log.Printf("[500] %s: read error: %v", absPath, err)
			http.Error(w, fmt.Sprintf("Internal Server Error: %v", err), http.StatusInternalServerError)
			return
		}

		html, err := pandoc.Convert(cfg, data, "")
		if err != nil {
			log.Printf("[500] %s -> %s: %v", urlPath, absPath, err)
			http.Error(w, fmt.Sprintf("Internal Server Error: %v", err), http.StatusInternalServerError)
			return
		}

		log.Printf("[200] %s -> %s", urlPath, absPath)
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	})

	addr := fmt.Sprintf(":%d", s.Port)
	log.Printf("Serve running at http://localhost%s/", addr)
	log.Printf("Root: %s", s.Root)
	return http.ListenAndServe(addr, mux)
}
