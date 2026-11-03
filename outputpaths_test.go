package main

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestOutputPaths(t *testing.T) {
	if os.Getenv("IRIS_TEST_SSG_PATHS") == "1" {
		os.Args = append([]string{"iris", "ssg"}, os.Args[len(os.Args)-2:]...)
		main()
		return
	}
	for _, mode := range []string{"ssg", "record"} {
		for _, name := range []string{"equal", "ancestor", "alias", "alias-parent", "missing", "file", "dangling"} {
			t.Run(mode+"/"+name, func(t *testing.T) {
				base := t.TempDir()
				input_dir := filepath.Join(base, "input")
				output_dir := filepath.Join(base, "output")
				if err := os.MkdirAll(input_dir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(output_dir, 0755); err != nil {
					t.Fatal(err)
				}
				sentinel := filepath.Join(input_dir, "keep.txt")
				output_sentinel := filepath.Join(output_dir, "keep.txt")
				for _, path := range []string{sentinel, output_sentinel} {
					if err := os.WriteFile(path, []byte("keep"), 0644); err != nil {
						t.Fatal(err)
					}
				}
				switch name {
				case "equal":
					output_dir = input_dir
				case "ancestor":
					output_dir = base
				case "alias":
					output_dir = filepath.Join(base, "alias")
					if err := os.Symlink(input_dir, output_dir); err != nil {
						t.Fatal(err)
					}
				case "alias-parent":
					alias := filepath.Join(base, "alias")
					if err := os.Symlink(base, alias); err != nil {
						t.Fatal(err)
					}
					output_dir = filepath.Join(alias, "input")
				case "missing":
					input_dir = filepath.Join(base, "missing")
				case "file":
					input_dir = sentinel
				case "dangling":
					output_dir = filepath.Join(base, "alias")
					if err := os.Symlink(filepath.Join(base, "missing"), output_dir); err != nil {
						t.Fatal(err)
					}
				}
				if mode == "record" {
					handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("handler called") })
					if err := recordServe(handler, input_dir, output_dir); err == nil {
						t.Fatal("accepted unsafe paths")
					}
				} else {
					cmd := exec.Command(os.Args[0], "-test.run=^TestOutputPaths$", "--", input_dir, output_dir)
					cmd.Env = append(os.Environ(), "IRIS_TEST_SSG_PATHS=1")
					if out, err := cmd.CombinedOutput(); err == nil {
						t.Fatalf("accepted unsafe paths: %s", out)
					}
				}
				for _, path := range []string{sentinel, output_sentinel} {
					if data, err := os.ReadFile(path); err != nil || string(data) != "keep" {
						t.Fatalf("lost %s: %s, %v", path, data, err)
					}
				}
			})
		}
	}
}

func TestOutputPathRootAndChild(t *testing.T) {
	base := t.TempDir()
	if _, _, err := validate_output_paths(base, string(filepath.Separator)); err == nil {
		t.Fatal("accepted filesystem root")
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(base, alias); err != nil {
		t.Fatal(err)
	}
	input_dir, output_dir, err := validate_output_paths(alias, filepath.Join(alias, "new", "output"))
	if err != nil {
		t.Fatal(err)
	}
	if input_dir != base || output_dir != filepath.Join(base, "new", "output") {
		t.Fatalf("paths: %s, %s", input_dir, output_dir)
	}
}
