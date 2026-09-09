package sourcepath

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrOutsideRoot = errors.New("path is outside root")

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

func (r *Resolver) Resolve(urlPath string, extensions []string) (string, string, error) {
	if r == nil || !strings.HasPrefix(urlPath, "/") {
		return "", "", ErrOutsideRoot
	}
	trimmed := strings.TrimPrefix(urlPath, "/")
	for _, part := range strings.Split(trimmed, "/") {
		if part == "." || part == ".." || strings.ContainsRune(part, 0) || strings.ContainsRune(part, '\\') {
			return "", "", ErrOutsideRoot
		}
	}
	for _, extension := range extensions {
		candidate := filepath.Join(r.root, filepath.FromSlash(trimmed+extension))
		realPath, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(r.root, realPath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", "", ErrOutsideRoot
		}
		info, err := os.Stat(realPath)
		if err == nil && !info.IsDir() {
			return realPath, extension, nil
		}
	}
	return "", "", os.ErrNotExist
}
