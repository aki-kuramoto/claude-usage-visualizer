// Package paths says where the application keeps its files.
//
// Everything lives under one directory in the home directory, the same on
// every OS: $HOME on macOS and Linux, %USERPROFILE% on Windows. Go's
// os.UserHomeDir already returns the right one for each.
package paths

import (
	"os"
	"path/filepath"
)

// Paths holds the directories and files the application uses.
type Paths struct {
	// Base is <home>/.app-data/claude-usage-visualizer.
	Base string
}

// Default returns the paths under the current user's home directory.
func Default() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	return Paths{Base: filepath.Join(home, ".app-data", "claude-usage-visualizer")}, nil
}

// Logs holds both the usage record and the operation log.
func (p Paths) Logs() string { return filepath.Join(p.Base, "logs") }

// Work is the directory claude runs in.
func (p Paths) Work() string { return filepath.Join(p.Base, "work") }

// Days is the file that says how much each day is used (see package days).
func (p Paths) Days() string { return filepath.Join(p.Base, "days.setay") }

// Settings is the settings file.
func (p Paths) Settings() string { return filepath.Join(p.Base, "settings.setay") }
