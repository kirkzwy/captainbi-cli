//go:build !windows

package cmd

import "os"

func replaceOutputFile(source, target string) error {
	return os.Rename(source, target)
}
