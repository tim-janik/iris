package editlink

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// resolveSourcePath resolves the absolute path to the source file for a given
// URL path and source root directory.
func resolveSourcePath(urlPath, srcRoot string) string {
	if !strings.HasPrefix(urlPath, "/") {
		urlPath = "/" + urlPath
	}

	for _, ext := range []string{"", ".md", ".adoc"} {
		candidate := filepath.Join(srcRoot, urlPath+ext)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}

	return ""
}

// handleEditQuery checks if the request has an "edl" query parameter.
// If so, it opens the editor at the specified line and returns true to signal
// the caller to short-circuit further processing.
func handleEditQuery(cfg Config, w http.ResponseWriter, r *http.Request, srcPath string) bool {
	edlStr := r.URL.Query().Get("edl")
	if edlStr == "" {
		return false
	}

	lineNum, err := strconv.Atoi(edlStr)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid line number: %s", edlStr), http.StatusInternalServerError)
		return true
	}

	if err := OpenEditor(cfg, srcPath, lineNum); err != nil {
		http.Error(w, fmt.Sprintf("Failed to open editor: %v", err), http.StatusInternalServerError)
		return true
	}

	w.WriteHeader(http.StatusOK)
	return true
}

// responseCapture wraps an http.ResponseWriter to capture the status code,
// headers, and body for later processing.
type responseCapture struct {
	http.ResponseWriter
	statusCode int
	headers    http.Header
	body       bytes.Buffer
}

func (rc *responseCapture) WriteHeader(code int) {
	rc.statusCode = code
	rc.headers = rc.ResponseWriter.Header().Clone()
}

func (rc *responseCapture) Write(b []byte) (int, error) {
	rc.body.Write(b)
	return len(b), nil
}

func (rc *responseCapture) flush(w http.ResponseWriter) {
	if rc.statusCode == 0 {
		rc.statusCode = http.StatusOK
	}
	if rc.headers != nil {
		for k, vv := range rc.headers {
			w.Header()[k] = vv
		}
	}
	w.WriteHeader(rc.statusCode)
	w.Write(rc.body.Bytes())
}

// Handler returns an HTTP middleware that wraps the given handler to provide
// editlink injection and ?edl= query handling.
func Handler(cfg Config, next http.Handler, srcRoot string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srcPath := resolveSourcePath(r.URL.Path, srcRoot)
		if srcPath == "" {
			// No source file found — pass through to next handler
			next.ServeHTTP(w, r)
			return
		}

		if handleEditQuery(cfg, w, r, srcPath) {
			return
		}

		// Capture the response
		rc := &responseCapture{ResponseWriter: w}
		next.ServeHTTP(rc, r)

		// If the response is HTML, inject edit links
		if strings.HasPrefix(rc.ResponseWriter.Header().Get("Content-Type"), "text/html") {
			modified, err := InjectEditLinks(rc.body.String(), srcPath)
			if err == nil {
				rc.body.Reset()
				rc.body.WriteString(modified)
			}
		}

		rc.flush(w)
	})
}
