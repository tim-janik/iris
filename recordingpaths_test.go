package main

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
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
