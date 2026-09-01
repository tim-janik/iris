// This Source Code Form is licensed MPL-2.0: http://mozilla.org/MPL/2.0
package main

import (
	"path/filepath"
	"strings"
)

func computeDirInfo(dir string) (dirName string, depth int, root string) {
	dirName = "/"
	if dir != "." {
		dirName = "/" + dir
	}
	if !strings.HasSuffix(dirName, "/") {
		dirName += "/"
	}
	if dir == "." {
		return dirName, 0, "."
	}
	depth = strings.Count(dir, "/") + 1
	return dirName, depth, strings.TrimRight(strings.Repeat("../", depth), "/")
}

// computePathInfo returns metadata for a file path (rel). The file adds one
// depth level below its containing directory: "2007/foo.md" sits inside
// "2007", so depth is 2, not 1.
func computePathInfo(rel string) (dirName string, depth int, root string) {
	dirName, depth, root = computeDirInfo(filepath.Dir(rel))
	depth++ // files are one level below their containing directory
	return
}

// computePathInfoForDir is like computePathInfo but takes a directory path
// directly (not a file path). Used for auto-generated dirindex pages.
// Depth starts at 0 for root and root uses ".." (no trailing slash).
func computePathInfoForDir(dir string) (dirName string, depth int, root string) {
	return computeDirInfo(dir)
}

// cleanURL strips the .html extension for clean URLs.
// index.html is converted to trailing slash (e.g. "2005/index.html" → "2005/").
func cleanURL(path string) string {
	if strings.HasSuffix(path, "/index.html") || path == "index.html" {
		dir := strings.TrimSuffix(path, "index.html")
		if dir == "" {
			return "/"
		}
		return dir
	}
	return strings.TrimSuffix(path, ".html")
}
