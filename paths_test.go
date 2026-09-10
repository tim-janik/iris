package main

import "testing"

func TestComputePathInfo(t *testing.T) {
	tests := []struct {
		path      string
		wantDir   string
		wantDepth int
		wantRoot  string
	}{
		{"page.md", "/", 1, "."},
		{"2025/post.md", "/2025/", 2, ".."},
		{"2025/docs/post.md", "/2025/docs/", 3, "../.."},
	}
	for _, test := range tests {
		dir, depth, root := computePathInfo(test.path)
		if dir != test.wantDir || depth != test.wantDepth || root != test.wantRoot {
			t.Errorf("computePathInfo(%q) = %q, %d, %q, want %q, %d, %q", test.path, dir, depth, root, test.wantDir, test.wantDepth, test.wantRoot)
		}
	}
}

func TestComputePathInfoForDir(t *testing.T) {
	tests := []struct {
		dir       string
		wantName  string
		wantDepth int
		wantRoot  string
	}{
		{".", "/", 0, "."},
		{"2025", "/2025/", 1, ".."},
		{"2025/docs", "/2025/docs/", 2, "../.."},
	}
	for _, test := range tests {
		name, depth, root := computePathInfoForDir(test.dir)
		if name != test.wantName || depth != test.wantDepth || root != test.wantRoot {
			t.Errorf("computePathInfoForDir(%q) = %q, %d, %q, want %q, %d, %q", test.dir, name, depth, root, test.wantName, test.wantDepth, test.wantRoot)
		}
	}
}
