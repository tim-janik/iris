package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tim-janik/iris/sourcepath"
)

func TestMetadataRouteEnumeratesDirectMarkdownChildren(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "tasks"), 0755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"tasks/open.md":           "---\nstatus: open\npriority: high\ndue: 2026-01-02\n---\nOpen\n",
		"tasks/no-title.md":       "No frontmatter\n",
		"tasks/.hidden.md":        "---\ntitle: hidden\n---\n",
		"tasks/nested.md/ignored": "not possible",
		"tasks/readme.txt":        "not markdown",
	}
	for name, content := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "tasks", "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks", "nested", "child.md"), []byte("---\ntitle: child\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/tasks/..~meta~?cmd=get-frontmatter-array", nil)
	rec := httptest.NewRecorder()
	(&Server{Root: root}).handleMetadataRoute(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("content type = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache control = %q", got)
	}
	var records []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &records); err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %#v", records)
	}
	byTitle := map[string]map[string]any{}
	for _, record := range records {
		byTitle[record["title"].(string)] = record
	}
	open := byTitle["open"]
	if open["url"] != "/tasks/open" || open["due"] != "2026-01-02" || open["priority"] != "high" {
		t.Errorf("open record = %#v", open)
	}
	if byTitle["no-title"]["url"] != "/tasks/no-title" {
		t.Errorf("synthesized record = %#v", byTitle["no-title"])
	}
}

func TestMetadataRouteEscapesFilenameURL(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "page#x.md"), []byte("---\ntitle: page\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/..~meta~?cmd=get-frontmatter-array", nil)
	rec := httptest.NewRecorder()
	(&Server{Root: root}).handleMetadataRoute(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"url":"/page%23x"`) {
		t.Errorf("metadata URL was not escaped: %s", rec.Body)
	}
}

func TestServeRedirectEscapesFilenameURL(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "page#x.md"), []byte("---\ntitle: page\n---\nbody\n"), 0644); err != nil {
		t.Fatal(err)
	}
	server := &Server{Root: root}
	handler, err := server.Handler()
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://example.com/page%23x.md", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Location"); got != "/page%23x" {
		t.Errorf("redirect location = %q", got)
	}
}

func TestMetadataRouteEmptyAndTraversal(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "empty"), 0755); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"/empty/..~meta~?cmd=get-frontmatter-array", "/../..~meta~?cmd=get-frontmatter-array"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		(&Server{Root: root}).handleMetadataRoute(rec, req)
		if path[1] == '.' {
			if rec.Code != http.StatusBadRequest {
				t.Errorf("traversal status = %d", rec.Code)
			}
			continue
		}
		if rec.Code != http.StatusOK || !reflect.DeepEqual(rec.Body.Bytes(), []byte("[]\n")) {
			t.Errorf("empty response = %d %q", rec.Code, rec.Body.String())
		}
	}
}

func TestMetadataRouteUsesPublicPathForSymlinkedRoot(t *testing.T) {
	realRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(realRoot, "page.md"), []byte("---\ntitle: page\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	linkParent := t.TempDir()
	rootLink := filepath.Join(linkParent, "root")
	if err := os.Symlink(realRoot, rootLink); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/..~meta~?cmd=get-frontmatter-array", nil)
	rec := httptest.NewRecorder()
	(&Server{Root: rootLink}).handleMetadataRoute(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"url":"/page"`) {
		t.Fatalf("symlinked root metadata = %d %s", rec.Code, rec.Body)
	}
}

func TestMetadataRoutePreservesInternalDirectoryAlias(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "posts")
	if err := os.Mkdir(realDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "page.md"), []byte("---\ntitle: page\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realDir, filepath.Join(root, "alias")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	metadata := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/alias/..~meta~?cmd=get-frontmatter-array", nil)
		rec := httptest.NewRecorder()
		(&Server{Root: root}).handleMetadataRoute(rec, req)
		return rec
	}
	if rec := metadata(); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"url":"/alias/page"`) {
		t.Fatalf("aliased metadata = %d %s", rec.Code, rec.Body)
	}

	req := httptest.NewRequest(http.MethodPost, "/alias/..~meta~?cmd=create-file&name=created.md", strings.NewReader("---\ntitle: created\n---\n"))
	rec := httptest.NewRecorder()
	(&Server{Root: root}).handleMetadataRoute(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("aliased create = %d %s", rec.Code, rec.Body)
	}
	if rec := metadata(); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"url":"/alias/created"`) {
		t.Fatalf("aliased metadata after create = %d %s", rec.Code, rec.Body)
	}
}

func TestCreateFileSuccess(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "tasks"), 0755); err != nil {
		t.Fatal(err)
	}
	body := "---\nstatus: open\npriority: high\n---\nNew issue body\n"
	req := httptest.NewRequest(http.MethodPost, "/tasks/..~meta~?cmd=create-file&name=new-issue.md", strings.NewReader(body))
	rec := httptest.NewRecorder()
	(&Server{Root: root}).handleMetadataRoute(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("content type = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache control = %q", got)
	}
	data, err := os.ReadFile(filepath.Join(root, "tasks", "new-issue.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != body {
		t.Errorf("written contents = %q, want %q", data, body)
	}
	// the new file must be visible to get-frontmatter-array immediately
	req = httptest.NewRequest(http.MethodGet, "/tasks/..~meta~?cmd=get-frontmatter-array", nil)
	rec = httptest.NewRecorder()
	(&Server{Root: root}).handleMetadataRoute(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "\"url\":\"/tasks/new-issue\"") {
		t.Errorf("metadata after create = %d %s", rec.Code, rec.Body)
	}
}

func TestCreateFileRejectsBadNames(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{".hidden.md", "sub/evil.md", `sub\evil.md`, "..", "../up.md", "notes.txt", "README", ""} {
		req := httptest.NewRequest(http.MethodPost, "/..~meta~?cmd=create-file&name="+name, strings.NewReader("x"))
		rec := httptest.NewRecorder()
		(&Server{Root: root}).handleMetadataRoute(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("name %q: status = %d, body = %s", name, rec.Code, rec.Body)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("directory not empty after rejected creates: %v", entries)
	}
}

func TestCreateFileConflictsWithExistingFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "exists.md"), []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/..~meta~?cmd=create-file&name=exists.md", strings.NewReader("overwrite attempt"))
	rec := httptest.NewRecorder()
	(&Server{Root: root}).handleMetadataRoute(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	data, err := os.ReadFile(filepath.Join(root, "exists.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original" {
		t.Errorf("existing file was modified: %q", data)
	}
}

func TestCreateFileRequiresPostAndExistingDirectory(t *testing.T) {
	root := t.TempDir()
	// GET is not allowed for create-file
	req := httptest.NewRequest(http.MethodGet, "/..~meta~?cmd=create-file&name=x.md", nil)
	rec := httptest.NewRecorder()
	(&Server{Root: root}).handleMetadataRoute(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET status = %d, body = %s", rec.Code, rec.Body)
	}
	// missing directory
	req = httptest.NewRequest(http.MethodPost, "/nope/..~meta~?cmd=create-file&name=x.md", strings.NewReader("x"))
	rec = httptest.NewRecorder()
	(&Server{Root: root}).handleMetadataRoute(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing dir status = %d, body = %s", rec.Code, rec.Body)
	}
	// traversal is rejected before any write
	req = httptest.NewRequest(http.MethodPost, "/../..~meta~?cmd=create-file&name=x.md", strings.NewReader("x"))
	rec = httptest.NewRecorder()
	(&Server{Root: root}).handleMetadataRoute(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("traversal status = %d, body = %s", rec.Code, rec.Body)
	}
}

func TestDashboardAsset(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/tasks/..~meta~?asset=dashboard.js", nil)
	rec := httptest.NewRecorder()
	(&Server{Root: t.TempDir()}).handleMetadataRoute(rec, req)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/javascript; charset=utf-8" {
		t.Fatalf("asset response = %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if len(rec.Body.Bytes()) == 0 {
		t.Fatal("empty dashboard asset")
	}
}

func TestPageAssetsRoute(t *testing.T) {
	tests := []struct {
		name        string
		server      Server
		wantStatus  int
		contentType string
		wantBody    string
	}{
		{
			name:        "mermaid script served",
			server:      Server{MermaidScript: []byte("mermaid-bytes")},
			wantStatus:  http.StatusOK,
			contentType: "application/javascript; charset=utf-8",
			wantBody:    "mermaid-bytes",
		},
		{
			name:        "highlight stylesheet served",
			server:      Server{HighlightStyle: []byte("css-bytes")},
			wantStatus:  http.StatusOK,
			contentType: "text/css; charset=utf-8",
			wantBody:    "css-bytes",
		},
		{
			name:       "unknown asset rejected",
			server:     Server{MermaidScript: []byte("mermaid-bytes")},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "unset asset rejected",
			server:     Server{},
			wantStatus: http.StatusNotFound,
		},
	}
	for _, test := range tests {
		asset := "mermaid.min.js"
		if test.name == "highlight stylesheet served" {
			asset = "github.min.css"
		}
		if test.name == "unknown asset rejected" {
			asset = "nope.js"
		}
		if test.name == "unset asset rejected" {
			asset = "highlight.min.js"
		}
		req := httptest.NewRequest(http.MethodGet, "/..~meta~?asset="+asset, nil)
		rec := httptest.NewRecorder()
		test.server.handleMetadataRoute(rec, req)
		if rec.Code != test.wantStatus {
			t.Errorf("%s: status = %d, want %d", test.name, rec.Code, test.wantStatus)
			continue
		}
		if test.wantBody != "" {
			if got := rec.Header().Get("Content-Type"); got != test.contentType {
				t.Errorf("%s: content type = %q, want %q", test.name, got, test.contentType)
			}
			if rec.Body.String() != test.wantBody {
				t.Errorf("%s: body = %q, want %q", test.name, rec.Body.String(), test.wantBody)
			}
		}
	}
}

func TestServeRootPrefix(t *testing.T) {
	tests := []struct {
		urlPath string
		want    string
	}{
		{urlPath: "/page", want: "."},
		{urlPath: "/", want: "."},
		{urlPath: "/posts/page", want: ".."},
		{urlPath: "/posts/2024/page", want: "../.."},
		{urlPath: "/posts/2024/deep/page", want: "../../.."},
	}
	for _, test := range tests {
		if got := serveRootPrefix(test.urlPath); got != test.want {
			t.Errorf("serveRootPrefix(%q) = %q, want %q", test.urlPath, got, test.want)
		}
	}
}

func TestActionAuthorization(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "tasks"), 0755); err != nil {
		t.Fatal(err)
	}
	server := &Server{Root: root}
	handler, err := server.Handler()
	if err != nil {
		t.Fatal(err)
	}
	request := func(origin, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "http://example.com/tasks/..~meta~?cmd=create-file&name=authorized.md", strings.NewReader("body"))
		req.Header.Set("Host", "example.com")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if token != "" {
			req.AddCookie(&http.Cookie{Name: "iris-action-token", Value: token})
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	if got := request("", ""); got.Code != http.StatusForbidden {
		t.Fatalf("missing credentials status = %d", got.Code)
	}
	if got := request("http://evil.example", server.actionToken); got.Code != http.StatusForbidden {
		t.Fatalf("foreign origin status = %d", got.Code)
	}
	if got := request("http://example.com/path", server.actionToken); got.Code != http.StatusForbidden {
		t.Fatalf("origin with path status = %d", got.Code)
	}
	if got := request("http://example.com", server.actionToken); got.Code != http.StatusCreated {
		t.Fatalf("authorized status = %d, body = %s", got.Code, got.Body)
	}
	if _, err := os.Stat(filepath.Join(root, "tasks", "authorized.md")); err != nil {
		t.Fatal(err)
	}
}

func TestActionCookieAndListenAddress(t *testing.T) {
	server := &Server{Root: t.TempDir(), Port: 9454}
	handler, err := server.Handler()
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://example.com/..~meta~?cmd=get-frontmatter-array", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	cookie := rec.Result().Cookies()
	if len(cookie) != 1 || cookie[0].Name != "iris-action-token" || !cookie[0].HttpOnly || cookie[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("action cookie = %#v", cookie)
	}
	if got := (&Server{Port: 9454}).listenAddress(); got != "127.0.0.1:9454" {
		t.Fatalf("default listen address = %q", got)
	}
	if got := (&Server{ListenHost: "0.0.0.0", Port: 9454}).listenAddress(); got != "0.0.0.0:9454" {
		t.Fatalf("explicit listen address = %q", got)
	}
}

func TestServeUsesConvertedAsciiDocTitle(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "about.adoc"), []byte("= About Converted\n\nPage body.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	server := &Server{Root: root}
	handler, err := server.Handler()
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://example.com/about", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "<title>About Converted</title>") {
		t.Fatalf("converted title missing: %s", rec.Body)
	}
}

func TestServePreservesExplicitAsciiDocTitle(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "about.adoc"), []byte("---\ntitle: Explicit Title\n---\n= Converted Heading\n\nPage body.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	server := &Server{Root: root}
	handler, err := server.Handler()
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://example.com/about", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "<title>Explicit Title</title>") {
		t.Fatalf("explicit title response = %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(body, "<h1>Converted Heading</h1>") {
		t.Fatalf("converted heading missing: %s", rec.Body)
	}
}

func TestHandlerRejectsOutsideSymlinksAndEncodedTraversal(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	if err := os.Mkdir(root, 0755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(base, "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	server := &Server{Root: root}
	handler, err := server.Handler()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		target string
		want   int
	}{
		{target: "/link.txt", want: http.StatusInternalServerError},
		{target: "/%2e%2e/outside.txt", want: http.StatusInternalServerError},
		{target: "/missing.txt", want: http.StatusNotFound},
	} {
		req := httptest.NewRequest(http.MethodGet, "http://example.com"+test.target, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != test.want {
			t.Errorf("%s status = %d, want %d, body = %s", test.target, rec.Code, test.want, rec.Body)
		}
	}
}

func TestResolveServeRoute(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"index.md":      "root",
		"docs/index.md": "docs",
		"foo.md":        "source",
		"foo.txt":       "static",
		"foo.txt.md":    "shadowed source",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "dir.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	resolver, err := sourcepath.New(root)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		path         string
		wantBase     string
		wantConvert  bool
		wantDirect   bool
		wantRedirect string
	}{
		{path: "/", wantBase: "index.md", wantConvert: true},
		{path: "/index", wantBase: "index.md", wantConvert: true},
		{path: "/docs/", wantBase: "index.md", wantConvert: true},
		{path: "/docs", wantRedirect: "/docs/"},
		{path: "/foo.txt", wantBase: "foo.txt"},
		{path: "/foo.md", wantBase: "foo.md", wantConvert: true, wantDirect: true},
		{path: "/dir.md", wantRedirect: "/dir.md/"},
	}
	for _, test := range tests {
		route, err := resolveServeRoute(resolver, test.path)
		if err != nil {
			t.Errorf("resolve %q: %v", test.path, err)
			continue
		}
		if route.redirect != test.wantRedirect {
			t.Errorf("resolve %q redirect = %q, want %q", test.path, route.redirect, test.wantRedirect)
		}
		if test.wantBase != "" && filepath.Base(route.path) != test.wantBase {
			t.Errorf("resolve %q path = %q, want base %q", test.path, route.path, test.wantBase)
		}
		if route.convert != test.wantConvert || route.directSource != test.wantDirect {
			t.Errorf("resolve %q flags = convert %v direct %v", test.path, route.convert, route.directSource)
		}
	}
}

func TestServeRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler, err := (&Server{Root: root}).Handler()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/..%2f" + filepath.Base(outside) + "/secret.txt", "/%2e%2e/secret.txt", "/.git/config"} {
		req := httptest.NewRequest(http.MethodGet, "http://example.test"+path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("request %q status = %d, body %q", path, rec.Code, rec.Body.String())
		}
	}
}

func TestServeStaticPolicy(t *testing.T) {
	root := t.TempDir()
	for name := range map[string]bool{
		"public.css":   true,
		"private.eml":  true,
		"private.log":  true,
		"private.json": true,
		"private.xml":  true,
		"private.go":   true,
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	handler, err := (&Server{Root: root}).Handler()
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]int{
		"public.css":   http.StatusOK,
		"private.eml":  http.StatusNotFound,
		"private.log":  http.StatusNotFound,
		"private.json": http.StatusNotFound,
		"private.xml":  http.StatusNotFound,
		"private.go":   http.StatusNotFound,
	} {
		req := httptest.NewRequest(http.MethodGet, "/"+name, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("GET /%s = %d, want %d", name, rec.Code, want)
		}
	}
}

func TestServeSourceRedirectPreservesQuery(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "page.md"), []byte("page"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler, err := (&Server{Root: root}).Handler()
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/page.md?view=full", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/page?view=full" {
		t.Fatalf("redirect = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	req = httptest.NewRequest(http.MethodGet, "/page.md?noredirect", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "page" {
		t.Fatalf("noredirect = %d %q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/markdown; charset=utf-8" {
		t.Fatalf("noredirect content type = %q", got)
	}
}
