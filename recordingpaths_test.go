package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tim-janik/iris/serve"
)

func TestRecordingLiteralPaths(t *testing.T) {
	base := t.TempDir()
	input_dir := filepath.Join(base, "input")
	output_dir := filepath.Join(base, "output")
	if err := os.Mkdir(input_dir, 0755); err != nil {
		t.Fatal(err)
	}
	names := []string{"100%.txt", "%2e%2e%2fescaped.txt", "%2Fabsolute.txt", "a%41.txt", "aA.txt", "sub/%23%3f.md"}
	for _, name := range names {
		path := filepath.Join(input_dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("source"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte(r.URL.Path)); err != nil {
			t.Error(err)
		}
	})
	if err := recordServe(handler, input_dir, output_dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if filepath.Ext(name) == ".md" {
			name = name[:len(name)-3]
		}
		data, err := os.ReadFile(filepath.Join(output_dir, name))
		if err != nil || string(data) != "/"+name {
			t.Errorf("%s: %q, %v", name, data, err)
		}
	}
	if _, err := os.Stat(filepath.Join(base, "escaped.txt")); !os.IsNotExist(err) {
		t.Fatalf("escaped output: %v", err)
	}
}

func TestRecordServeDirectoryIndexesAndPrivatePaths(t *testing.T) {
	root := t.TempDir()
	output := t.TempDir()
	for name, content := range map[string]string{
		"index.md":            "# Root page",
		"docs/index.md":       "# Docs page",
		"docs/child.txt":      "child",
		"static/index.html":   "Static page",
		".private/secret.txt": "secret",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "outside.txt")); err != nil {
		t.Fatal(err)
	}
	handler, err := (&serve.Server{Root: root}).Handler()
	if err != nil {
		t.Fatal(err)
	}
	if err := recordServe(handler, root, output); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"index.html":        "Root page",
		"docs/index.html":   "Docs page",
		"docs/index":        "Docs page",
		"docs/child.txt":    "child",
		"static/index.html": "Static page",
	} {
		if data, err := os.ReadFile(filepath.Join(output, name)); err != nil || !strings.Contains(string(data), want) {
			t.Errorf("%s = %q, %v", name, data, err)
		}
	}
	for _, name := range []string{".private", "outside.txt"} {
		if _, err := os.Stat(filepath.Join(output, name)); !os.IsNotExist(err) {
			t.Errorf("recorded rejected path %s: %v", name, err)
		}
	}
}
