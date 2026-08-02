package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
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
