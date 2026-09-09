package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func resolve_output_path(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	if _, stat_err := os.Lstat(path); !os.IsNotExist(stat_err) {
		return "", err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return "", err
	}
	resolved, err = resolve_output_path(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(path)), nil
}

func validate_output_paths(input_dir, output_dir string) (string, string, error) {
	input_abs, err := filepath.Abs(input_dir)
	if err != nil {
		return "", "", err
	}
	input_abs, err = filepath.EvalSymlinks(input_abs)
	if err != nil {
		return "", "", fmt.Errorf("input directory: %w", err)
	}
	info, err := os.Stat(input_abs)
	if err != nil {
		return "", "", err
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("input is not a directory: %s", input_dir)
	}
	output_abs, err := filepath.Abs(output_dir)
	if err != nil {
		return "", "", err
	}
	output_abs, err = resolve_output_path(output_abs)
	if err != nil {
		return "", "", fmt.Errorf("output directory: %w", err)
	}
	rel, err := filepath.Rel(output_abs, input_abs)
	if err != nil {
		return "", "", err
	}
	if rel == "." || filepath.IsLocal(rel) {
		return "", "", fmt.Errorf("output directory %s contains input directory %s", output_dir, input_dir)
	}
	return input_abs, output_abs, nil
}
