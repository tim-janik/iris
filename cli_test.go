package main

import (
	"bytes"
	"log"
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

func TestRunSSGNestedOutputThroughSymlink(t *testing.T) {
	input_dir := t.TempDir()
	alias := filepath.Join(t.TempDir(), "repo")
	if err := os.Symlink(input_dir, alias); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(input_dir, "page.txt"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}
	args := ssgArgs{clearOutput: true, inputDir: alias, outputDir: filepath.Join(alias, "site"), workers: 1, now: time.Now()}
	for _, clear := range []bool{true, false, true} {
		args.clearOutput = clear
		if err := runSSG(args); err != nil {
			t.Fatal(err)
		}
		if data, err := os.ReadFile(filepath.Join(args.outputDir, "page.txt")); err != nil || string(data) != "content" {
			t.Fatalf("built page = %q, %v", data, err)
		}
		if _, err := os.Stat(filepath.Join(args.outputDir, "site")); !os.IsNotExist(err) {
			t.Fatalf("output was included in input: %v", err)
		}
	}
}

func TestRunSSGGenerationFailurePreservesOutput(t *testing.T) {
	input_dir := t.TempDir()
	output_dir := filepath.Join(input_dir, "site")
	if err := os.MkdirAll(filepath.Join(output_dir, "rss2.xml"), 0755); err != nil {
		t.Fatal(err)
	}
	old_file := filepath.Join(output_dir, "old.txt")
	if err := os.WriteFile(old_file, []byte("old output"), 0644); err != nil {
		t.Fatal(err)
	}
	err := runSSG(ssgArgs{inputDir: input_dir, outputDir: output_dir, workers: 1, now: time.Now()})
	if err == nil {
		t.Fatal("build succeeded with a blocked feed path")
	}
	if data, err := os.ReadFile(old_file); err != nil || string(data) != "old output" {
		t.Fatalf("old output = %q, %v", data, err)
	}
	if entries, err := os.ReadDir(input_dir); err != nil || len(entries) != 1 || entries[0].Name() != "site" {
		t.Fatalf("staging directory was not removed: %v, %v", entries, err)
	}
}

func TestInstallOutputRestoresBackup(t *testing.T) {
	root := t.TempDir()
	output_dir := filepath.Join(root, "site")
	if err := os.Mkdir(output_dir, 0755); err != nil {
		t.Fatal(err)
	}
	old_file := filepath.Join(output_dir, "old.txt")
	if err := os.WriteFile(old_file, []byte("old output"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := installOutput(filepath.Join(root, "missing"), output_dir); err == nil {
		t.Fatal("installed missing staging directory")
	}
	if data, err := os.ReadFile(old_file); err != nil || string(data) != "old output" {
		t.Fatalf("restored output = %q, %v", data, err)
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

func TestInstallOutputCleanupWarning(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "site")
	locked := filepath.Join(output, "locked")
	stage := filepath.Join(root, "stage")
	if err := os.MkdirAll(locked, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "old.txt"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err == nil && entry.IsDir() {
				os.Chmod(path, 0755)
			}
			return nil
		})
	})
	if err := os.WriteFile(filepath.Join(locked, "probe"), nil, 0644); err == nil {
		t.Skip("directory permissions are not enforced")
	}
	if err := os.Mkdir(stage, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "new.txt"), []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	writer := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(writer)
	if err := installOutput(stage, output); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(output, "new.txt")); err != nil || string(data) != "new" {
		t.Fatalf("installed output = %q, %v", data, err)
	}
	backups, err := filepath.Glob(filepath.Join(root, ".site.old-*"))
	if err != nil || len(backups) != 1 || !strings.Contains(logs.String(), backups[0]) {
		t.Fatalf("backup warning = %q, backups = %v, %v", logs.String(), backups, err)
	}
}

func TestRunSSGMergeWithReadOnlyParent(t *testing.T) {
	input := t.TempDir()
	parent := t.TempDir()
	output := filepath.Join(parent, "site")
	if err := os.Mkdir(output, 0755); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		filepath.Join(input, "page.txt"): "new",
		filepath.Join(output, "old.txt"): "old",
	} {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	before, err := os.Stat(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(parent, 0755)
	if err := runSSG(ssgArgs{inputDir: input, outputDir: output, workers: 1, now: time.Now()}); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(output)
	if err != nil || !os.SameFile(before, after) {
		t.Fatalf("output directory was replaced: %v", err)
	}
	for name, want := range map[string]string{"page.txt": "new", "old.txt": "old"} {
		if data, err := os.ReadFile(filepath.Join(output, name)); err != nil || string(data) != want {
			t.Errorf("%s = %q, %v", name, data, err)
		}
	}
}

func TestWalkFilesSkipsBuildDirectories(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"page.txt", "site/old.txt", ".site.tmp-123/page.txt", ".site.old-456/page.txt", "docs/.site.tmp-123/page.txt"} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}
	filter, err := globstar.NewFilter([]string{"**"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	files, err := walkFiles(root, filepath.Join(root, "site"), filter)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"docs/.site.tmp-123/page.txt", "page.txt"}; !reflect.DeepEqual(files, want) {
		t.Errorf("walkFiles = %v, want %v", files, want)
	}
}
