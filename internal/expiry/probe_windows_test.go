//go:build windows

package expiry

import "golang.org/x/sys/windows"

// isTerminal tells whether standard input and output are a console, which
// on a pseudo console they are.
func isTerminal() bool {
	var mode uint32
	in := windows.GetConsoleMode(windows.Stdin, &mode)
	out := windows.GetConsoleMode(windows.Stdout, &mode)
	return in == nil && out == nil
}
