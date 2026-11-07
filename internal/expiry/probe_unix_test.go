//go:build !windows

package expiry

import "os"

// isTerminal tells whether standard input and output are a terminal, which
// on a pseudo terminal they are.
func isTerminal() bool {
	for _, f := range []*os.File{os.Stdin, os.Stdout} {
		st, err := f.Stat()
		if err != nil || st.Mode()&os.ModeCharDevice == 0 {
			return false
		}
	}
	return true
}
