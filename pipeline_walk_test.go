package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tim-janik/iris/globstar"
)

func TestWalkFilesFiltersFilesAfterTraversingDirectories(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"docs/guide.md", "docs/image.png", "assets/site.css", "secret/hidden.md", ".private/page.md"} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}
	filter, err := globstar.NewFilter([]string{"**/*.md"}, []string{"secret/**"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := walkFiles(root, filepath.Join(root, "out"), filter)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"docs/guide.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("walkFiles = %#v, want %#v", got, want)
	}
}

func TestWalkFilesTraversesAssetOnlyDirectories(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "assets", "site.css")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("css"), 0644); err != nil {
		t.Fatal(err)
	}
	filter, err := globstar.NewFilter([]string{"**/*.md", "assets/**"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := walkFiles(root, filepath.Join(root, "out"), filter)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"assets/site.css"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("walkFiles = %#v, want %#v", got, want)
	}
}
