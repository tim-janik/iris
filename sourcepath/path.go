package sourcepath

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrOutsideRoot = errors.New("path is outside root")
	ErrInvalidPath = errors.New("invalid source path")
	ErrPrivatePath = errors.New("private source path")
	ErrNotRegular  = errors.New("source path is not a regular file")
)

type Resolver struct {
	root string
}

func New(root string) (*Resolver, error) {
	if strings.ContainsRune(root, 0) {
		return nil, ErrInvalidPath
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(realRoot)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, ErrNotRegular
	}
	return &Resolver{root: realRoot}, nil
}

func (r *Resolver) Resolve(urlPath string, extensions []string) (string, string, error) {
	for _, extension := range extensions {
		path, info, err := r.ResolvePath(urlPath + extension)
		if err == nil {
			if info.IsDir() {
				continue
			}
			return path, extension, nil
		}
		if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, ErrNotRegular) {
			return "", "", err
		}
	}
	return "", "", os.ErrNotExist
}

func (r *Resolver) ResolvePath(urlPath string) (string, os.FileInfo, error) {
	if r == nil {
		return "", nil, ErrOutsideRoot
	}
	parts, err := splitURLPath(urlPath)
	if err != nil {
		return "", nil, err
	}
	candidate := r.root
	if len(parts) != 0 {
		candidate = filepath.Join(candidate, filepath.FromSlash(strings.Join(parts, "/")))
	}
	realPath, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", nil, err
	}
	rel, err := filepath.Rel(r.root, realPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", nil, fmt.Errorf("%w: %w", ErrOutsideRoot, ErrPrivatePath)
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

func ResolveRegular(r *Resolver, urlPath string) (string, os.FileInfo, error) {
	path, info, err := r.ResolvePath(urlPath)
	if err != nil {
		return path, info, err
	}
	if info.IsDir() {
		return path, info, ErrNotRegular
	}
	return path, info, nil
}

func splitURLPath(urlPath string) ([]string, error) {
	decoded, err := url.PathUnescape(urlPath)
	if err != nil {
		return nil, ErrInvalidPath
	}
	if !strings.HasPrefix(decoded, "/") {
		decoded = "/" + decoded
	}
	if strings.ContainsRune(decoded, 0) {
		return nil, ErrInvalidPath
	}

	var parts []string
	for _, part := range strings.Split(strings.Trim(decoded, "/"), "/") {
		if part == "" {
			continue
		}
		if part == "." || part == ".." || strings.ContainsRune(part, '\\') {
			return nil, fmt.Errorf("%w: %w", ErrInvalidPath, ErrOutsideRoot)
		}
		if strings.HasPrefix(part, ".") {
			return nil, ErrPrivatePath
		}
		parts = append(parts, part)
	}
	return parts, nil
}
