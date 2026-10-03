package main

import (
	"bytes"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tim-janik/iris/globstar"
	"github.com/tim-janik/iris/serve"
)

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
	if err := os.Mkdir(output_dir, 0755); err != nil {
		t.Fatal(err)
	}
	old_file := filepath.Join(output_dir, "old.txt")
	if err := os.WriteFile(old_file, []byte("old output"), 0644); err != nil {
		t.Fatal(err)
	}
	template_dir := t.TempDir()
	if err := os.CopyFS(template_dir, os.DirFS("templates")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(template_dir, "rss2.xml"), []byte(`{{define "rss2"}}{{.MissingField}}{{end}}`), 0644); err != nil {
		t.Fatal(err)
	}
	for _, clear := range []bool{true, false} {
		err := runSSG(ssgArgs{clearOutput: clear, inputDir: input_dir, outputDir: output_dir, templateDir: template_dir, workers: 1, now: time.Now()})
		if err == nil || !strings.Contains(err.Error(), "MissingField") {
			t.Fatalf("build with broken feed template: %v", err)
		}
		if data, err := os.ReadFile(old_file); err != nil || string(data) != "old output" {
			t.Fatalf("old output = %q, %v", data, err)
		}
		if dirs, err := filepath.Glob(filepath.Join(input_dir, ".site.tmp-*")); err != nil || len(dirs) != 0 {
			t.Fatalf("staging directory was not removed: %v, %v", dirs, err)
		}
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
	stage := filepath.Join(output_dir, "stage")
	if err := os.Mkdir(stage, 0755); err != nil {
		t.Fatal(err)
	}
	if err := installOutput(stage, output_dir); err == nil {
		t.Fatal("installed staging directory moved inside backup")
	}
	if data, err := os.ReadFile(old_file); err != nil || string(data) != "old output" {
		t.Fatalf("restored output = %q, %v", data, err)
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
	if os.Getenv("IRIS_TEST_SSG_UMASK") != "1" {
		cmd := exec.Command("sh", "-c", `umask 077; exec "$@"`, "sh", os.Args[0], "-test.run=^TestRunSSGOutputMode$")
		cmd.Env = append(os.Environ(), "IRIS_TEST_SSG_UMASK=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("restricted umask: %v\n%s", err, out)
		}
		return
	}
	for _, clear := range []bool{true, false} {
		for _, existing := range []bool{true, false} {
			input := t.TempDir()
			output := filepath.Join(t.TempDir(), "output")
			want := os.FileMode(0700)
			if existing {
				want = 0750 | os.ModeSetuid | os.ModeSetgid | os.ModeSticky
				if err := os.Mkdir(output, want); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(output, want); err != nil {
					t.Fatal(err)
				}
			}
			want_file := os.FileMode(0600)
			if existing && !clear {
				want_file = 0640
				feed := filepath.Join(output, "rss2.xml")
				if err := os.WriteFile(feed, []byte("old"), want_file); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(feed, want_file); err != nil {
					t.Fatal(err)
				}
			}
			if err := runSSG(ssgArgs{clearOutput: clear, inputDir: input, outputDir: output, workers: 1, now: time.Now()}); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(output)
			if err != nil || info.Mode()&(os.ModePerm|os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != want {
				t.Fatalf("output mode (clear=%v, existing=%v): %v, %v; want %o", clear, existing, info, err, want)
			}
			info, err = os.Stat(filepath.Join(output, "rss2.xml"))
			if err != nil || info.Mode().Perm() != want_file {
				t.Fatalf("file mode: %v, %v; want %o", info, err, want_file)
			}
		}
	}
}

func TestCopyOutputTreeRejectsSymlinkDestinations(t *testing.T) {
	for _, test := range []struct {
		name      string
		inside    bool
		directory bool
	}{
		{"outside file", false, false},
		{"inside file", true, false},
		{"outside directory", false, true},
		{"inside directory", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := t.TempDir()
			output := t.TempDir()
			target_dir := t.TempDir()
			if test.inside {
				target_dir = filepath.Join(output, "kept")
				if err := os.Mkdir(target_dir, 0755); err != nil {
					t.Fatal(err)
				}
			}
			target := filepath.Join(target_dir, "file.txt")
			if err := os.WriteFile(target, []byte("keep"), 0644); err != nil {
				t.Fatal(err)
			}
			link, rel, link_target := "alias.txt", "alias.txt", target
			if test.directory {
				link, rel, link_target = "alias", "alias/file.txt", target_dir
			}
			if test.inside {
				link_target = filepath.Join("kept", "file.txt")
				if test.directory {
					link_target = "kept"
				}
			}
			if err := os.Symlink(link_target, filepath.Join(output, link)); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(source, rel)
			if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, []byte("new"), 0644); err != nil {
				t.Fatal(err)
			}
			if err := copyOutputTree(source, output); err == nil {
				t.Error("installed through a symlink")
			}
			if data, err := os.ReadFile(target); err != nil || string(data) != "keep" {
				t.Fatalf("ungenerated target changed: %q, %v", data, err)
			}
		})
	}
}

func TestCopyOutputTreeRejectsFIFO(t *testing.T) {
	mkfifo, err := exec.LookPath("mkfifo")
	if err != nil {
		t.Skip("mkfifo not available")
	}
	source, output := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "page.txt"), []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	pipe := filepath.Join(output, "page.txt")
	if out, err := exec.Command(mkfifo, pipe).CombinedOutput(); err != nil {
		t.Fatalf("mkfifo: %v, %s", err, out)
	}
	// Keep the pipe open so the pre-fix write cannot hang the test.
	reader, err := os.OpenFile(pipe, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err := copyOutputTree(source, output); err == nil {
		t.Fatal("installed into a FIFO")
	}
}

func TestCopyOutputTreePreservesHardLinkTargets(t *testing.T) {
	for _, inside := range []bool{true, false} {
		source, output, target_dir := t.TempDir(), t.TempDir(), t.TempDir()
		if inside {
			target_dir = output
		}
		target := filepath.Join(target_dir, "keep.txt")
		if err := os.WriteFile(target, []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
		old_time := time.Unix(1000000000, 0)
		if err := os.Chtimes(target, old_time, old_time); err != nil {
			t.Fatal(err)
		}
		alias := filepath.Join(output, "page.txt")
		if err := os.Link(target, alias); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, "page.txt"), []byte("new"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := copyOutputTree(source, output); err != nil {
			t.Fatal(err)
		}
		for path, want := range map[string]string{alias: "new", target: "keep"} {
			if data, err := os.ReadFile(path); err != nil || string(data) != want {
				t.Errorf("%s = %q, %v; want %q", path, data, err, want)
			}
		}
		if info, err := os.Stat(alias); err != nil || info.Mode().Perm() != 0600 {
			t.Errorf("replacement mode = %v, %v; want 600", info, err)
		}
		info, err := os.Stat(target)
		if err != nil || !info.ModTime().Equal(old_time) {
			t.Errorf("ungenerated target timestamp changed: %v, %v", info, err)
		}
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
	old_file := filepath.Join(output, "old.txt")
	old_time := time.Unix(1000000000, 0)
	if err := os.Chtimes(old_file, old_time, old_time); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(old_file, 0444); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("old.txt", filepath.Join(output, "alias.txt")); err != nil {
		t.Fatal(err)
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
	info, err := os.Stat(old_file)
	if err != nil || !info.ModTime().Equal(old_time) || info.Mode().Perm() != 0444 {
		t.Fatalf("untouched file changed: %v, %v", info, err)
	}
	if target, err := os.Readlink(filepath.Join(output, "alias.txt")); err != nil || target != "old.txt" {
		t.Fatalf("untouched symlink changed: %q, %v", target, err)
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
