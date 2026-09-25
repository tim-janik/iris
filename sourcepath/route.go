package sourcepath

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type Route struct {
	Path         string
	Source       bool
	DirectSource bool
	Redirect     string
}

func is_source_path(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".adoc"
}

func (r *Resolver) ResolveRoute(url_path string) (Route, error) {
	url_path = "/" + strings.TrimLeft(url_path, "/")
	path, info, err := r.ResolvePath(url_path)
	var candidates []string
	if err == nil {
		if !info.IsDir() {
			source := is_source_path(path)
			return Route{Path: path, Source: source, DirectSource: source && is_source_path(url_path)}, nil
		}
		if !strings.HasSuffix(url_path, "/") {
			return Route{Redirect: url_path + "/"}, nil
		}
		candidates = []string{url_path + "index.html", url_path + "index.md", url_path + "index.adoc"}
	} else {
		if !os.IsNotExist(err) && !errors.Is(err, ErrNotRegular) {
			return Route{}, err
		}
		if strings.HasSuffix(url_path, "/") {
			return Route{}, os.ErrNotExist
		}
		candidates = []string{url_path + ".md", url_path + ".adoc"}
	}
	for _, candidate := range candidates {
		path, _, err := ResolveRegular(r, candidate)
		if err == nil {
			return Route{Path: path, Source: is_source_path(path)}, nil
		}
		if !os.IsNotExist(err) && !errors.Is(err, ErrNotRegular) {
			return Route{}, err
		}
	}
	return Route{}, os.ErrNotExist
}
