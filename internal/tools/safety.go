package tools

import (
	"fmt"
	"path/filepath"
	"strings"
)

func resolvePath(workdir, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required")
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("absolute paths are not allowed")
	}
	absWorkdir, err := filepath.Abs(workdir)
	if err != nil {
		return "", err
	}
	absPath, err := filepath.Abs(filepath.Join(absWorkdir, path))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absWorkdir, absPath)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workdir")
	}
	return absPath, nil
}

func displayPath(workdir, path string) string {
	rel, err := filepath.Rel(workdir, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

func capOutput(text string, limit int) string {
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[:limit] + "\n[truncated]"
}
