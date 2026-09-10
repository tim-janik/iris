package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tim-janik/iris/globstar"
	"github.com/tim-janik/iris/serve"
)

func TestWalkFilesKeepsNestedGlobMatches(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"post.md", filepath.Join("2025", "deep.md"), filepath.Join("private", "secret.md")} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	filter, err := globstar.NewFilter([]string{"**/*.md"}, []string{"private/**"})
	if err != nil {
		t.Fatal(err)
	}
	files, err := walkFiles(root, filepath.Join(t.TempDir(), "out"), filter)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"2025/deep.md", "post.md"}; !reflect.DeepEqual(files, want) {
		t.Errorf("walkFiles() = %v, want %v", files, want)
	}
}

func TestValidateSSGPathsRejectsOverlap(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "input")
	if err := os.Mkdir(input, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{input, filepath.Join(root, "input", "out"), root} {
		err := validateSSGArgs(ssgArgs{inputDir: input, outputDir: output, workers: 1, now: time.Now()})
		if err == nil || !strings.Contains(err.Error(), "overlap") {
			t.Errorf("validateSSGArgs output %q = %v, want overlap error", output, err)
		}
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(input, alias); err == nil {
		err := validateSSGArgs(ssgArgs{inputDir: input, outputDir: filepath.Join(alias, "out"), workers: 1})
		if err == nil || !strings.Contains(err.Error(), "overlap") {
			t.Errorf("validateSSGArgs symlink alias = %v, want overlap error", err)
		}
	}
}

func TestSSGOutputCollisions(t *testing.T) {
	for _, files := range [][]string{
		{"foo.md", "foo.adoc"},
		{"foo.md", "foo.html"},
		{"rss2.xml"},
	} {
		if err := validateOutputCollisions(files); err == nil {
			t.Errorf("validateOutputCollisions(%v) succeeded", files)
		} else {
			for _, file := range files {
				if !strings.Contains(err.Error(), file) {
					t.Errorf("collision error %q does not name %q", err, file)
				}
			}
		}
	}
}

func TestRunSSGFailurePreservesOutput(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "input")
	output := filepath.Join(root, "output")
	if err := os.MkdirAll(input, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(output, 0o755); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(output, "old.txt")
	if err := os.WriteFile(old, []byte("old output"), 0o644); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "bad.toml")
	if err := os.WriteFile(config, []byte("not = [valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runSSG(ssgArgs{inputDir: input, outputDir: output, configFile: config, workers: 1, now: time.Now()})
	if err == nil {
		t.Fatal("runSSG succeeded with invalid config")
	}
	if data, readErr := os.ReadFile(old); readErr != nil || string(data) != "old output" {
		t.Fatalf("old output after failed build = %q, %v", data, readErr)
	}
}

func TestRunSSGOutputMode(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "input")
	output := filepath.Join(root, "output")
	if err := os.Mkdir(input, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(output, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := runSSG(ssgArgs{clearOutput: false, inputDir: input, outputDir: output, workers: 1, now: time.Now()}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(output)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Errorf("output directory mode = %o, want 755", got)
	}
}

func TestCopyOutputTreePreservesMode(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")
	if err := os.MkdirAll(filepath.Join(src, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(src, "nested", "page.txt")
	if err := os.WriteFile(file, []byte("page"), 0o601); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(file, 0o601); err != nil {
		t.Fatal(err)
	}
	if err := copyOutputTree(src, dst); err != nil {
		t.Fatal(err)
	}
	copy := filepath.Join(dst, "nested", "page.txt")
	info, err := os.Stat(copy)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o601 {
		t.Errorf("copied mode = %o, want 601", got)
	}
	data, err := os.ReadFile(copy)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "page" {
		t.Errorf("copied content = %q, want page", data)
	}
}

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
