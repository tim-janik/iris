package sourcepath

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveContainedFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "page.md"), []byte("page"), 0644); err != nil {
		t.Fatal(err)
	}
	resolver, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	path, extension, err := resolver.Resolve("/page", []string{"", ".md", ".adoc"})
	if err != nil || extension != ".md" || path != filepath.Join(root, "page.md") {
		t.Fatalf("resolve = %q, %q, %v", path, extension, err)
	}
}

func TestResolveSymlinksInsideAndOutside(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	outside := filepath.Join(base, "outside.md")
	if err := os.Mkdir(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "inside.md"), []byte("inside"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("outside"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(base, "outside-dir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "outside-dir", "nested.txt"), []byte("outside"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "inside.md"), filepath.Join(root, "inside-link.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "sub"), filepath.Join(root, "sub-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "nested.txt"), []byte("inside"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "outside-link.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "outside-dir"), filepath.Join(root, "outside-dir")); err != nil {
		t.Fatal(err)
	}
	resolver, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolver.Resolve("/inside-link.md", []string{""}); err != nil {
		t.Fatalf("internal link: %v", err)
	}
	if _, _, err := resolver.Resolve("/sub-link/nested.txt", []string{""}); err != nil {
		t.Fatalf("internal directory link: %v", err)
	}
	if _, _, err := resolver.Resolve("/outside-link.md", []string{""}); !errors.Is(err, ErrOutsideRoot) {
		t.Fatalf("outside link error = %v", err)
	}
	if _, _, err := resolver.Resolve("/outside-dir/nested.txt", []string{""}); !errors.Is(err, ErrOutsideRoot) {
		t.Fatalf("outside directory link error = %v", err)
	}
}

func TestResolveRejectsTraversal(t *testing.T) {
	resolver, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/../outside.md", "/nested/../../outside.md", "/nested\\outside.md"} {
		if _, _, err := resolver.Resolve(path, []string{""}); !errors.Is(err, ErrOutsideRoot) {
			t.Errorf("%q error = %v", path, err)
		}
	}
}
