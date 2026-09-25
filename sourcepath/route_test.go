package sourcepath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRoute(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"index.md":          "root",
		"docs/index.md":     "docs",
		"foo.md":            "source",
		"foo.txt":           "static",
		"foo.txt.md":        "shadowed source",
		"fallback.adoc":     "fallback",
		"nested/index.adoc": "nested fallback",
		"static/index.html": "static index",
		"static/index.md":   "shadowed index",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"dir.md", "fallback.md", "nested/index.md"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(root, "foo.md"), filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	resolver, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		path         string
		wantBase     string
		wantConvert  bool
		wantDirect   bool
		wantRedirect string
	}{
		{path: "/", wantBase: "index.md", wantConvert: true},
		{path: "/index", wantBase: "index.md", wantConvert: true},
		{path: "/docs/", wantBase: "index.md", wantConvert: true},
		{path: "/docs", wantRedirect: "/docs/"},
		{path: "//docs", wantRedirect: "/docs/"},
		{path: "/foo.txt", wantBase: "foo.txt"},
		{path: "/foo.md", wantBase: "foo.md", wantConvert: true, wantDirect: true},
		{path: "/dir.md", wantRedirect: "/dir.md/"},
		{path: "/alias", wantBase: "foo.md", wantConvert: true},
		{path: "/fallback", wantBase: "fallback.adoc", wantConvert: true},
		{path: "/nested/", wantBase: "index.adoc", wantConvert: true},
		{path: "/static/", wantBase: "index.html"},
	}
	for _, test := range tests {
		route, err := resolver.ResolveRoute(test.path)
		if err != nil {
			t.Errorf("resolve %q: %v", test.path, err)
			continue
		}
		if route.Redirect != test.wantRedirect {
			t.Errorf("resolve %q redirect = %q, want %q", test.path, route.Redirect, test.wantRedirect)
		}
		if test.wantBase != "" && filepath.Base(route.Path) != test.wantBase {
			t.Errorf("resolve %q path = %q, want base %q", test.path, route.Path, test.wantBase)
		}
		if route.Source != test.wantConvert || route.DirectSource != test.wantDirect {
			t.Errorf("resolve %q flags = convert %v direct %v", test.path, route.Source, route.DirectSource)
		}
	}
}
