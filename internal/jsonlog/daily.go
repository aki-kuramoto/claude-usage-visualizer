// Package jsonlog appends JSON Lines to one file per day.
//
// Both records this application keeps (the usage record and the operation
// log) are written this way. Files are split by the UTC date, so the split
// stays consistent when the machine's time zone changes. Nothing here ever
// deletes or rotates a file; old files stay until a person removes them.
package jsonlog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Daily appends to Dir/Prefix-YYYY-MM-DD.jsonl.
type Daily struct {
	Dir    string
	Prefix string
}

// Path returns the file that a line written at t goes into.
func (d Daily) Path(t time.Time) string {
	return filepath.Join(d.Dir, fmt.Sprintf("%s-%s.jsonl", d.Prefix, t.UTC().Format(time.DateOnly)))
}

// Append writes v as one line to the file for t, creating the directory and
// the file when they are missing.
//
// The file is opened for each line rather than held open. Lines are written
// every few minutes at most, and this way a file removed by hand is simply
// created again.
func (d Daily) Append(t time.Time, v any) error {
	line, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("jsonlog: %w", err)
	}
	line = append(line, '\n')

	if err := os.MkdirAll(d.Dir, 0o755); err != nil {
		return fmt.Errorf("jsonlog: %w", err)
	}
	f, err := os.OpenFile(d.Path(t), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("jsonlog: %w", err)
	}
	// One write per line, so that a crash leaves at most the last line
	// incomplete.
	if _, err := f.Write(line); err != nil {
		f.Close()
		return fmt.Errorf("jsonlog: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("jsonlog: %w", err)
	}
	return nil
}
