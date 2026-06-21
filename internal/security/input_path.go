package security

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const MaxInputFileSize int64 = 10 << 20

var ErrUnsafeInputPath = errors.New("unsafe input path")

// SafeInputPath resolves a regular input file while keeping reads inside cwd.
func SafeInputPath(path string) (string, error) {
	if path == "" || filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: use a relative path inside the current working directory", ErrUnsafeInputPath)
	}
	cleaned := filepath.Clean(path)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: parent-directory traversal is not allowed", ErrUnsafeInputPath)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	root, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(root, cleaned))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: symbolic link resolves outside the current working directory", ErrUnsafeInputPath)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: input must be a regular file", ErrUnsafeInputPath)
	}
	if info.Size() > MaxInputFileSize {
		return "", fmt.Errorf("%w: input file exceeds %d bytes", ErrUnsafeInputPath, MaxInputFileSize)
	}
	return resolved, nil
}
