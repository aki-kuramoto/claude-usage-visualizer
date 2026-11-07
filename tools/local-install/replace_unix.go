//go:build !darwin && !windows

package main

import "os"

// replace moves the new executable into place. A rename within one directory
// is atomic, and a running process keeps the old file open.
func replace(tmp, dest string) error {
	return os.Rename(tmp, dest)
}
