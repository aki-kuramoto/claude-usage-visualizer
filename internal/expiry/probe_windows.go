//go:build windows

package expiry

import "context"

// Probe is not available on Windows yet: it needs ConPTY, the Windows
// pseudo console, which this package does not use yet.
func Probe(ctx context.Context, path, dir string, env []string) (Result, error) {
	return Result{Status: StatusUnsupported}, nil
}
