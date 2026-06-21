package lockfile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	defaultWait  = 30 * time.Second
	defaultStale = 2 * time.Minute
)

func Acquire(ctx context.Context, path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(defaultWait)
	for {
		if err := os.Mkdir(path, 0o700); err == nil {
			_ = os.WriteFile(filepath.Join(path, "acquired_at"), []byte(time.Now().Format(time.RFC3339Nano)), 0o600)
			return func() { _ = os.RemoveAll(path) }, nil
		} else if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) > defaultStale {
			_ = os.RemoveAll(path)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for lock %s", path)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
