package editlink

import (
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tim-janik/iris/sourcepath"
)

func TestEditActionRequiresPostAndToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page.md")
	if err := os.WriteFile(path, []byte("# Heading\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Cmd: "true", Token: "secret"}
	request := func(method, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/page?edl=1", nil)
		if token != "" {
			req.AddCookie(&http.Cookie{Name: "iris-action-token", Value: token})
		}
		rec := httptest.NewRecorder()
		if !handleEditQuery(cfg, rec, req, path) {
			t.Fatal("edit query was not handled")
		}
		return rec
	}
	if got := request(http.MethodGet, "secret"); got.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d", got.Code)
	}
	if got := request(http.MethodPost, ""); got.Code != http.StatusForbidden {
		t.Fatalf("missing token status = %d", got.Code)
	}
	if got := request(http.MethodPost, "secret"); got.Code != http.StatusOK {
		t.Fatalf("authorized status = %d", got.Code)
	}
}

func TestInjectedEditLinksUsePost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page.md")
	if err := os.WriteFile(path, []byte("# Heading\n"), 0644); err != nil {
		t.Fatal(err)
	}
	htmlText, err := InjectEditLinks("<html><body><h1>Heading</h1></body></html>", path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html.UnescapeString(htmlText), "method:'POST'") {
		t.Fatalf("edit link does not use POST: %s", htmlText)
	}
}

func TestEditActionUsesServedSource(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"index.html", "index.md", "page.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	resolver, err := sourcepath.New(root)
	if err != nil {
		t.Fatal(err)
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
	handler := Handler(Config{Cmd: "true"}, next, resolver)
	for target, want := range map[string]int{
		"/?edl=1":     http.StatusMethodNotAllowed,
		"/page?edl=1": http.StatusOK,
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, target, nil))
		if rec.Code != want {
			t.Errorf("POST %s = %d, want %d", target, rec.Code, want)
		}
	}
}
