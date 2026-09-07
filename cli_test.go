package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tim-janik/iris/serve"
)

func TestRecordServe(t *testing.T) {
	root := t.TempDir()
	write := func(name, content string) {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("page.md", "# Title\n\n```mermaid\nflowchart TD\n  a --> b\n```\n")
	write("sub/post.md", "post text\n")
	write("sub/data.txt", "raw text\n")
	write("dup.txt", "raw dup\n")
	write("dup.txt.md", "converted dup\n")
	write("binary.xyz", "x")

	server := serve.Server{Root: root}
	handler, err := server.Handler()
	if err != nil {
		t.Fatalf("Handler(): %v", err)
	}

	out := t.TempDir()
	if err := recordServe(handler, root, out); err != nil {
		t.Fatalf("recordServe(): %v", err)
	}

	read := func(name string) string {
		data, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return string(data)
	}
	page := read("page")
	for _, want := range []string{"<h1", "Title", `<pre class="mermaid">`} {
		if !strings.Contains(page, want) {
			t.Errorf("page missing %q", want)
		}
	}
	if got := read(filepath.Join("sub", "post")); !strings.Contains(got, "post text") {
		t.Errorf("sub/post = %q, want post text", got)
	}
	if got := read(filepath.Join("sub", "data.txt")); got != "raw text\n" {
		t.Errorf("sub/data.txt = %q, want passthrough copy", got)
	}
	if got := read("dup.txt"); got != "raw dup\n" {
		t.Errorf("dup.txt = %q, want raw dup (serve resolves the file as-is first)", got)
	}
	if _, err := os.Stat(filepath.Join(out, "binary.xyz")); !os.IsNotExist(err) {
		t.Error("binary.xyz recorded, but serve answers 404 for it")
	}

	// Recording into a directory inside root must skip it during the walk.
	outInRoot := filepath.Join(root, "nested", "out")
	if err := recordServe(handler, root, outInRoot); err != nil {
		t.Fatalf("recordServe(nested): %v", err)
	}
	if _, err := os.Stat(filepath.Join(outInRoot, "nested")); !os.IsNotExist(err) {
		t.Error("nested out dir recorded itself")
	}
	if got := read2(outInRoot, "page"); !strings.Contains(got, "Title") {
		t.Errorf("nested recording page = %q, want converted page", got)
	}

	// Re-recording into the same dir replaces its previous contents.
	write("late.md", "late text\n")
	if err := recordServe(handler, root, out); err != nil {
		t.Fatalf("recordServe(again): %v", err)
	}
	if got := read2(out, "late"); !strings.Contains(got, "late text") {
		t.Errorf("late = %q, want recorded", got)
	}
}

func read2(dir, name string) string {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return "read error: " + err.Error()
	}
	return string(data)
}
