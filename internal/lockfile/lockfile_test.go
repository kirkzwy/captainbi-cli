package lockfile

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireReclaimsStaleLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-3 * time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	release, err := Acquire(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(path, "acquired_at")); err != nil {
		t.Fatalf("new lock metadata missing: %v", err)
	}
	release()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("lock directory remains after release: %v", err)
	}
}
