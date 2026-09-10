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
	path, info, err := resolver.ResolvePath("/page.md")
	if err != nil || info.IsDir() || path != filepath.Join(root, "page.md") {
		t.Fatalf("resolve = %q, %#v, %v", path, info, err)
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
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "sub"), filepath.Join(root, "sub-link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "nested.txt"), []byte("inside"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "outside-link.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(filepath.Join(base, "outside-dir"), filepath.Join(root, "outside-dir")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	resolver, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolver.ResolvePath("/inside-link.md"); err != nil {
		t.Fatalf("internal link: %v", err)
	}
	if _, _, err := resolver.ResolvePath("/sub-link/nested.txt"); err != nil {
		t.Fatalf("internal directory link: %v", err)
	}
	if _, _, err := resolver.ResolvePath("/outside-link.md"); !errors.Is(err, ErrOutsideRoot) || !errors.Is(err, ErrPrivatePath) {
		t.Fatalf("outside link error = %v", err)
	}
	if _, _, err := resolver.ResolvePath("/outside-dir/nested.txt"); !errors.Is(err, ErrOutsideRoot) || !errors.Is(err, ErrPrivatePath) {
		t.Fatalf("outside directory link error = %v", err)
	}
}

func TestResolveRejectsTraversal(t *testing.T) {
	resolver, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/../outside.md", "/nested/../../outside.md", "/nested\\outside.md"} {
		if _, _, err := resolver.ResolvePath(path); !errors.Is(err, ErrOutsideRoot) || !errors.Is(err, ErrInvalidPath) {
			t.Errorf("%q error = %v", path, err)
		}
	}
}

func TestResolverRejectsTraversalAndPrivatePaths(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "page.md"), []byte("page"), 0644); err != nil {
		t.Fatal(err)
	}
	resolver, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/../page.md", "/.git/config", "/page\\other"} {
		_, _, err := resolver.ResolvePath(path)
		if err == nil {
			t.Errorf("Resolve(%q) succeeded", path)
		}
	}
	_, _, err = resolver.ResolvePath("/../page.md")
	if !errors.Is(err, ErrInvalidPath) {
		t.Errorf("Resolve traversal error = %v, want ErrInvalidPath", err)
	}
	_, _, err = resolver.ResolvePath("/.git/config")
	if !errors.Is(err, ErrPrivatePath) {
		t.Errorf("Resolve private error = %v, want ErrPrivatePath", err)
	}
}

func TestResolverAllowsPercentEscapesInFilename(t *testing.T) {
	root := t.TempDir()
	name := "literal%2e.md"
	if err := os.WriteFile(filepath.Join(root, name), []byte("page"), 0644); err != nil {
		t.Fatal(err)
	}
	resolver, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	path, info, err := resolver.ResolvePath("/" + name)
	if err != nil || info.IsDir() || path != filepath.Join(root, name) {
		t.Errorf("ResolvePath(%q) = %q, %#v, %v", name, path, info, err)
	}
}

func TestResolverRejectsSymlinksAndSpecialFiles(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "page.md"), []byte("page"), 0644); err != nil {
		t.Fatal(err)
	}
	resolver, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = resolver.ResolvePath("/link.txt")
	if !errors.Is(err, ErrPrivatePath) {
		t.Errorf("Resolve symlink error = %v, want ErrPrivatePath", err)
	}
	_, _, err = resolver.ResolvePath("/page.md")
	if err != nil {
		t.Fatalf("Resolve regular file: %v", err)
	}
}

func TestResolverAllowsDirectories(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "docs"), 0755); err != nil {
		t.Fatal(err)
	}
	resolver, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	path, info, err := resolver.ResolvePath("/docs/")
	if err != nil || !info.IsDir() || path != filepath.Join(root, "docs") {
		t.Errorf("Resolve directory = %q, %#v, %v", path, info, err)
	}
}
