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
	handleMetadataRoute(rec, req, root)
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

func TestMetadataRouteEmptyAndTraversal(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "empty"), 0755); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"/empty/..~meta~?cmd=get-frontmatter-array", "/../..~meta~?cmd=get-frontmatter-array"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handleMetadataRoute(rec, req, root)
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

func TestCreateFileSuccess(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "tasks"), 0755); err != nil {
		t.Fatal(err)
	}
	body := "---\nstatus: open\npriority: high\n---\nNew issue body\n"
	req := httptest.NewRequest(http.MethodPost, "/tasks/..~meta~?cmd=create-file&name=new-issue.md", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handleMetadataRoute(rec, req, root)
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
	handleMetadataRoute(rec, req, root)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "\"url\":\"/tasks/new-issue\"") {
		t.Errorf("metadata after create = %d %s", rec.Code, rec.Body)
	}
}

func TestCreateFileRejectsBadNames(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{".hidden.md", "sub/evil.md", `sub\evil.md`, "..", "../up.md", "notes.txt", "README", ""} {
		req := httptest.NewRequest(http.MethodPost, "/..~meta~?cmd=create-file&name="+name, strings.NewReader("x"))
		rec := httptest.NewRecorder()
		handleMetadataRoute(rec, req, root)
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
	handleMetadataRoute(rec, req, root)
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
	handleMetadataRoute(rec, req, root)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET status = %d, body = %s", rec.Code, rec.Body)
	}
	// missing directory
	req = httptest.NewRequest(http.MethodPost, "/nope/..~meta~?cmd=create-file&name=x.md", strings.NewReader("x"))
	rec = httptest.NewRecorder()
	handleMetadataRoute(rec, req, root)
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing dir status = %d, body = %s", rec.Code, rec.Body)
	}
	// traversal is rejected before any write
	req = httptest.NewRequest(http.MethodPost, "/../..~meta~?cmd=create-file&name=x.md", strings.NewReader("x"))
	rec = httptest.NewRecorder()
	handleMetadataRoute(rec, req, root)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("traversal status = %d, body = %s", rec.Code, rec.Body)
	}
}

func TestDashboardAsset(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/tasks/..~meta~?asset=dashboard.js", nil)
	rec := httptest.NewRecorder()
	handleMetadataRoute(rec, req, t.TempDir())
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/javascript; charset=utf-8" {
		t.Fatalf("asset response = %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if len(rec.Body.Bytes()) == 0 {
		t.Fatal("empty dashboard asset")
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
