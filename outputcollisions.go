package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

func outputPathForInput(rel string) string {
	for _, ext := range []string{".md", ".adoc"} {
		if strings.HasSuffix(rel, ext) {
			return strings.TrimSuffix(rel, ext) + ".html"
		}
	}
	return rel
}

func validateOutputCollisions(paths []string) error {
	outputs := make(map[string]string)
	add := func(path, owner string) error {
		path = filepath.ToSlash(filepath.Clean(path))
		if previous, ok := outputs[path]; ok {
			return fmt.Errorf("output collision at %s: %s and %s", path, previous, owner)
		}
		for parent := path; parent != "."; parent = filepath.ToSlash(filepath.Dir(parent)) {
			if previous, ok := outputs[parent]; ok {
				return fmt.Errorf("output path %s is below file %s from %s", path, parent, previous)
			}
		}
		prefix := path + "/"
		for existing, previous := range outputs {
			if strings.HasPrefix(existing, prefix) {
				return fmt.Errorf("output file %s from %s is above %s from %s", path, owner, existing, previous)
			}
		}
		outputs[path] = owner
		return nil
	}

	for _, rel := range paths {
		if err := add(outputPathForInput(rel), "source "+rel); err != nil {
			return err
		}
	}
	for _, generated := range []string{
		"assets/highlight.js/highlight.min.js",
		"assets/highlight.js/styles/github.min.css",
		"assets/mermaid/mermaid.min.js",
		"rss2.xml",
		"atom.xml",
		"sitemap.xml",
	} {
		if err := add(generated, "generated "+generated); err != nil {
			return err
		}
	}

	for _, rel := range paths {
		if !strings.HasSuffix(rel, ".md") && !strings.HasSuffix(rel, ".adoc") {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(outputPathForInput(rel)))
		index := filepath.Join(dir, "index.html")
		if dir == "." {
			index = "index.html"
		}
		if _, exists := outputs[filepath.ToSlash(index)]; exists {
			continue
		}
		if err := add(index, "generated "+filepath.ToSlash(index)); err != nil {
			return err
		}
	}
	return nil
}
