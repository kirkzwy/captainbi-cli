package security

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSafeInputPath(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("params.json", []byte(`{"page":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SafeInputPath("params.json"); err != nil {
		t.Fatalf("safe relative file rejected: %v", err)
	}
	if _, err := SafeInputPath(filepath.Join(outside, "secret")); !errors.Is(err, ErrUnsafeInputPath) {
		t.Fatalf("absolute path should be unsafe, got %v", err)
	}
	if _, err := SafeInputPath("../secret"); !errors.Is(err, ErrUnsafeInputPath) {
		t.Fatalf("parent traversal should be unsafe, got %v", err)
	}
	outsideFile := filepath.Join(outside, "secret")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideFile, "linked.json"); err != nil {
		t.Fatal(err)
	}
	if _, err := SafeInputPath("linked.json"); !errors.Is(err, ErrUnsafeInputPath) {
		t.Fatalf("escaping symlink should be unsafe, got %v", err)
	}
}
