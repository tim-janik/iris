package sourcepath

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrOutsideRoot = errors.New("path is outside root")
	ErrPrivatePath = errors.New("private source path")
	ErrNotRegular  = errors.New("source path is not a regular file")
)

type Resolver struct {
	root string
}

func New(root string) (*Resolver, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	realRoot, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(realRoot)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("root is not a directory: %s", root)
	}
	return &Resolver{root: realRoot}, nil
}

func (r *Resolver) ResolvePath(urlPath string) (string, os.FileInfo, error) {
	if r == nil {
		return "", nil, ErrOutsideRoot
	}
	if err := validateURLPath(urlPath); err != nil {
		return "", nil, err
	}
	candidate := filepath.Join(r.root, filepath.FromSlash(strings.Trim(urlPath, "/")))
	realPath, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", nil, err
	}
	rel, err := filepath.Rel(r.root, realPath)
	if err != nil || !filepath.IsLocal(rel) {
		return "", nil, ErrOutsideRoot
	}
	if rel != "." {
		if err := validateURLPath(filepath.ToSlash(rel)); err != nil {
			return "", nil, err
		}
	}
	info, err := os.Stat(realPath)
	if err != nil {
		return "", nil, err
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return realPath, info, ErrNotRegular
	}
	return realPath, info, nil
}

func validateURLPath(urlPath string) error {
	if strings.ContainsRune(urlPath, 0) {
		return ErrOutsideRoot
	}
	for _, part := range strings.Split(urlPath, "/") {
		if part == "." || part == ".." || strings.ContainsRune(part, '\\') {
			return ErrOutsideRoot
		}
		if strings.HasPrefix(part, ".") {
			return ErrPrivatePath
		}
	}
	return nil
}
