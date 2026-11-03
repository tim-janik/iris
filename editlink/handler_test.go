package editlink

import (
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
